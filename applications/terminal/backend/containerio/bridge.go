//go:build linux

// Package containerio relays Terminal's requests into the project container.
// One `lxc exec <container> -- remote-terminal bridge` process carries every
// request for an installation as JSON lines, and the container's
// remote-terminal service answers them. The shells belong to that service, so
// this process, and the bridge with it, can restart without ending any.
package containerio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"futrx.local/catalog/applications/terminal/backend/config"
)

// maxFrameBytes matches the container bridge's line limit.
const maxFrameBytes = 4 << 20

// Request and Response are the bridge's line format, shared with
// backend/container/internal/bridge.
type Request struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query,omitempty"`
	Body   []byte `json:"body,omitempty"`
}

type Response struct {
	ID     uint64 `json:"id"`
	Status int    `json:"status"`
	Body   []byte `json:"body,omitempty"`
}

var errBridgeExited = errors.New("the terminal bridge into the project container exited")

// BridgeCommand returns how to start the bridge into container. The command
// is fixed; browser input travels only inside its JSON lines.
func BridgeCommand(container string) func() *exec.Cmd {
	return func() *exec.Cmd {
		return exec.Command("lxc", "exec", container, "--", config.ContainerCommand, "bridge")
	}
}

// Bridge sends requests over one bridge process, starting another when the
// last one has exited.
type Bridge struct {
	start  func() *exec.Cmd
	nextID atomic.Uint64

	mu      sync.Mutex
	current *process
}

func NewBridge(start func() *exec.Cmd) *Bridge {
	return &Bridge{start: start}
}

// Do sends request and waits for its answer, the bridge exiting, or ctx.
func (b *Bridge) Do(ctx context.Context, request Request) (Response, error) {
	current, err := b.process()
	if err != nil {
		return Response{}, err
	}
	request.ID = b.nextID.Add(1)
	answer, err := current.send(request)
	if err != nil {
		return Response{}, err
	}
	defer current.forget(request.ID)
	select {
	case response := <-answer:
		return response, nil
	case <-current.exited:
		return Response{}, current.failure()
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
}

// Warm starts the bridge ahead of the first request, so opening a terminal
// does not wait for `lxc exec`. A failure is retried by the next request.
func (b *Bridge) Warm() {
	_, _ = b.process()
}

// Close stops the current bridge process.
func (b *Bridge) Close() {
	b.mu.Lock()
	current := b.current
	b.current = nil
	b.mu.Unlock()
	if current != nil {
		current.stop()
	}
}

func (b *Bridge) process() (*process, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current != nil && !b.current.done() {
		return b.current, nil
	}
	started, err := startProcess(b.start())
	if err != nil {
		return nil, fmt.Errorf("start the terminal bridge: %w", err)
	}
	b.current = started
	return started, nil
}

type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *tail
	exited chan struct{}

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan Response
	err     error
}

func startProcess(cmd *exec.Cmd) (*process, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &process{cmd: cmd, stdin: stdin, stderr: stderr, exited: make(chan struct{}), pending: map[uint64]chan Response{}}
	go p.read(stdout)
	return p, nil
}

func (p *process) read(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxFrameBytes)
	for scanner.Scan() {
		var response Response
		if json.Unmarshal(scanner.Bytes(), &response) != nil {
			continue
		}
		p.mu.Lock()
		answer := p.pending[response.ID]
		delete(p.pending, response.ID)
		p.mu.Unlock()
		if answer != nil {
			answer <- response
		}
	}
	// A bridge that stopped making sense is replaced rather than kept.
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	waitErr := p.cmd.Wait()
	p.mu.Lock()
	p.err = errBridgeExited
	if message := strings.TrimSpace(p.stderr.String()); message != "" {
		p.err = fmt.Errorf("%w: %s", errBridgeExited, message)
	} else if waitErr != nil {
		p.err = fmt.Errorf("%w: %v", errBridgeExited, waitErr)
	}
	p.mu.Unlock()
	close(p.exited)
}

func (p *process) send(request Request) (<-chan Response, error) {
	line, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(line) >= maxFrameBytes {
		return nil, errors.New("terminal request is too large")
	}
	answer := make(chan Response, 1)
	p.mu.Lock()
	if p.done() {
		p.mu.Unlock()
		return nil, p.failure()
	}
	p.pending[request.ID] = answer
	p.mu.Unlock()

	p.writeMu.Lock()
	_, err = p.stdin.Write(append(line, '\n'))
	p.writeMu.Unlock()
	if err != nil {
		p.forget(request.ID)
		p.stop()
		return nil, fmt.Errorf("%w: %v", errBridgeExited, err)
	}
	return answer, nil
}

func (p *process) forget(id uint64) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *process) done() bool {
	select {
	case <-p.exited:
		return true
	default:
		return false
	}
}

func (p *process) failure() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err == nil {
		return errBridgeExited
	}
	return p.err
}

func (p *process) stop() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// tail keeps the start of the bridge's diagnostics without ever failing a write.
type tail struct {
	limit int
	mu    sync.Mutex
	text  strings.Builder
}

func (t *tail) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if room := t.limit - t.text.Len(); room > 0 {
		t.text.Write(data[:min(room, len(data))])
	}
	return len(data), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.text.String()
}
