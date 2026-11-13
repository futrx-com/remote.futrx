//go:build linux

package containerio

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *diagnosticPrefix
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
	stderr := &diagnosticPrefix{limit: 4096}
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

// diagnosticPrefix keeps the start of the bridge's diagnostics without ever failing a write.
type diagnosticPrefix struct {
	limit int
	mu    sync.Mutex
	text  strings.Builder
}

func (t *diagnosticPrefix) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if room := t.limit - t.text.Len(); room > 0 {
		t.text.Write(data[:min(room, len(data))])
	}
	return len(data), nil
}

func (t *diagnosticPrefix) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.text.String()
}
