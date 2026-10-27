package kimi

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

//go:embed bridge.mjs
var serverBridge string

type bridgeFrame struct {
	Type    string          `json:"type"`
	ID      int             `json:"id"`
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
	Event   json.RawMessage `json:"event"`
}

type serverError struct {
	Code    int
	Message string
}

func (e *serverError) Error() string { return fmt.Sprintf("Kimi API (%d): %s", e.Code, e.Message) }

type serverTransport struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	frames  chan bridgeFrame
	done    chan error
	nextID  int
	onEvent func(json.RawMessage) error
}

func startServerTransport(ctx context.Context, cmd *exec.Cmd) (*serverTransport, error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	// The bridge consumes the CLI's startup banner, which contains a private
	// bearer token. Only normalized protocol frames cross this boundary.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		in.Close()
		return nil, fmt.Errorf("start Kimi bridge: %w", err)
	}
	p := &serverTransport{cmd: cmd, stdin: in, frames: make(chan bridgeFrame, configconstants.KimiBridgeFrameQueueSize), done: make(chan error, 1)}
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, configconstants.KimiBridgeScanBufferBytes), configconstants.KimiBridgeMaxFrameBytes)
		for scanner.Scan() {
			var frame bridgeFrame
			if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
				p.frames <- bridgeFrame{Type: "fatal", Message: "Invalid response from Kimi bridge"}
				break
			}
			p.frames <- frame
		}
		if err := scanner.Err(); err != nil {
			p.frames <- bridgeFrame{Type: "fatal", Message: err.Error()}
		}
		close(p.frames)
		p.done <- cmd.Wait()
	}()
	frame, err := p.read(ctx)
	if err != nil || frame.Type != "ready" {
		p.close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("Kimi bridge did not initialize")
	}
	return p, nil
}

func (p *serverTransport) write(value any) error { return json.NewEncoder(p.stdin).Encode(value) }
func (p *serverTransport) read(ctx context.Context) (bridgeFrame, error) {
	select {
	case <-ctx.Done():
		return bridgeFrame{}, ctx.Err()
	case frame, ok := <-p.frames:
		if !ok {
			return bridgeFrame{}, errors.New("Kimi bridge closed unexpectedly")
		}
		if frame.Type == "fatal" {
			return frame, errors.New(frame.Message)
		}
		return frame, nil
	}
}

func (p *serverTransport) request(ctx context.Context, request bridgeRequest, result any) error {
	p.nextID++
	id := p.nextID
	request.ID = id
	if err := p.write(request); err != nil {
		return err
	}
	for {
		frame, err := p.read(ctx)
		if err != nil {
			return err
		}
		if frame.Type == "event" {
			if p.onEvent != nil {
				if err := p.onEvent(frame.Event); err != nil {
					return err
				}
			}
			continue
		}
		if frame.Type != "response" || frame.ID != id {
			return errors.New("Unexpected Kimi API response")
		}
		if frame.Code != 0 {
			return &serverError{frame.Code, frame.Msg}
		}
		if result != nil && len(frame.Data) > 0 {
			return json.Unmarshal(frame.Data, result)
		}
		return nil
	}
}

func (p *serverTransport) api(ctx context.Context, method, path string, body, result any) error {
	request := bridgeRequest{Type: "http", Method: method, Path: path, Body: body}
	return p.request(ctx, request, result)
}

func (p *serverTransport) close() error {
	var failure error
	_ = p.stdin.Close()
	timer := time.NewTimer(configconstants.KimiBridgeCloseTimeout)
	defer timer.Stop()
	frames := p.frames
	killed := false
	for {
		// Wait can finish while final frames are still buffered. Consume them
		// before accepting the process exit, including late failures/usage.
		var done <-chan error
		if frames == nil {
			done = p.done
		}
		select {
		case err := <-done:
			if failure != nil {
				return failure
			}
			if err != nil {
				return fmt.Errorf("Kimi bridge exited: %w", err)
			}
			return nil
		case frame, ok := <-frames: // Drain while the CLI shuts down.
			if !ok {
				frames = nil
			} else if frame.Type == "fatal" {
				failure = errors.New(frame.Message)
			} else if frame.Type == "event" && p.onEvent != nil {
				if err := p.onEvent(frame.Event); err != nil {
					failure = err
				}
			}
		case <-timer.C:
			if killed {
				return errors.New("Kimi bridge did not shut down")
			}
			killed = true
			_ = p.cmd.Process.Kill()
			timer.Reset(configconstants.KimiBridgeKillTimeout)
		}
	}
}
