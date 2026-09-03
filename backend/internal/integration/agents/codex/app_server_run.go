package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

type appServerRun struct {
	ctx context.Context
	cmd *exec.Cmd
	req agent.RunRequest

	emit           func(agent.Event)
	stdin          io.WriteCloser
	scanner        *bufio.Scanner
	stderrDone     chan string
	write          func(any) error
	eventParser    *appServerEventParser
	requestHandler *appServerRequestHandler
	terminal       bool
	runFailed      bool
	protocolErr    error
}

// runAppServer owns one Codex app-server process for one Remote turn. A fresh
// transport can still resume or fork a persisted Codex thread, so no daemon is
// required between user messages.
func runAppServer(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	emit func(agent.Event),
) error {
	return newAppServerRun(ctx, cmd, req, emit).execute()
}

func newAppServerRun(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	emit func(agent.Event),
) *appServerRun {
	return &appServerRun{ctx: ctx, cmd: cmd, req: req, emit: emit}
}

func (run *appServerRun) execute() error {
	if err := run.start(); err != nil {
		return err
	}
	if err := run.initialize(); err != nil {
		run.abortInitialization()
		return err
	}
	run.consumeOutput()
	return run.finish()
}

func (run *appServerRun) start() error {
	stdin, err := run.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := run.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := run.cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := run.cmd.Start(); err != nil {
		return fmt.Errorf("spawn codex app-server: %w", err)
	}

	run.stdin = stdin
	run.stderrDone = make(chan string, 1)
	go captureAppServerStderr(stderr, run.req.ConversationID, run.stderrDone)
	encoder := json.NewEncoder(stdin)
	run.write = encoder.Encode
	run.eventParser = newAppServerEventParser(run.req)
	run.requestHandler = newAppServerRequestHandler(run.req, run.emit, run.write)
	run.scanner = bufio.NewScanner(stdout)
	run.scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return nil
}

func (run *appServerRun) initialize() error {
	return run.write(map[string]any{
		"method": "initialize",
		"id":     appServerInitializeRequestID,
		"params": map[string]any{
			"clientInfo": map[string]string{
				"name":    "remote-futrx",
				"title":   "Remote",
				"version": "1",
			},
			"capabilities": map[string]bool{"experimentalApi": true},
		},
	})
}

func (run *appServerRun) abortInitialization() {
	_ = run.cmd.Process.Kill()
	_ = run.cmd.Wait()
	<-run.stderrDone
}

func (run *appServerRun) consumeOutput() {
	for run.scanner.Scan() {
		line := append([]byte(nil), run.scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var envelope appServerEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			log.Printf("codex[%s] app-server parse: %v", run.req.ConversationID, err)
			continue
		}
		if !run.handleEnvelope(envelope) {
			break
		}
	}
}

func (run *appServerRun) handleEnvelope(envelope appServerEnvelope) bool {
	if envelope.Method != "" && len(envelope.ID) > 0 {
		run.protocolErr = run.requestHandler.Answer(envelope)
		return run.protocolErr == nil
	}
	if envelope.Method != "" {
		run.handleNotification(envelope)
		return true
	}

	responseID, ok := rpcResponseID(envelope.ID)
	if !ok {
		return true
	}
	if envelope.Error != nil {
		run.protocolErr = run.responseError(responseID, envelope.Error.Message)
		return false
	}
	run.handleResponse(responseID, envelope.Result)
	return run.protocolErr == nil
}

func (run *appServerRun) handleNotification(envelope appServerEnvelope) {
	for _, event := range run.eventParser.ParseNotification(envelope.Method, envelope.Params) {
		run.emit(event)
		if event.Type == agent.EventRunCompleted || event.Type == agent.EventRunFailed {
			run.terminal = true
			run.runFailed = event.Type == agent.EventRunFailed
			_ = run.stdin.Close()
		}
	}
}

func (run *appServerRun) responseError(responseID int, message string) error {
	message = strings.TrimSpace(message)
	if responseID == appServerThreadRequestID && run.req.ResumeID != "" && isMissingCodexThread(message) {
		return fmt.Errorf("%w: %s", agent.ErrSessionNotFound, message)
	}
	return fmt.Errorf("codex app-server request %d: %s", responseID, message)
}

func (run *appServerRun) handleResponse(responseID int, resultJSON json.RawMessage) {
	switch responseID {
	case appServerInitializeRequestID:
		if err := run.write(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
			run.protocolErr = err
			return
		}
		request := buildAppServerThreadRequest(run.req)
		run.protocolErr = run.write(map[string]any{
			"method": request.Method,
			"id":     appServerThreadRequestID,
			"params": request.Params,
		})

	case appServerThreadRequestID:
		var result appServerThreadResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			run.protocolErr = fmt.Errorf("decode Codex thread response: %w", err)
			return
		}
		if result.Thread.ID == "" || result.Model == "" {
			run.protocolErr = errors.New("Codex app-server returned an incomplete thread")
			return
		}
		// The server resolves aliases such as "auto" to the concrete model.
		// Carry that model into the completion usage persisted for rebuilds.
		run.eventParser.req.Model = result.Model
		if result.Thread.ID != run.req.ResumeID {
			run.emit(agent.Event{
				T:              time.Now().UnixMilli(),
				Type:           agent.EventSessionUpdated,
				Provider:       agent.ProviderCodex,
				ConversationID: run.req.ConversationID,
				SessionID:      result.Thread.ID,
				Model:          result.Model,
			})
		}
		run.protocolErr = run.write(map[string]any{
			"method": "turn/start",
			"id":     appServerTurnRequestID,
			"params": buildAppServerTurnParams(run.req, result.Thread.ID, result.Model),
		})

	case appServerTurnRequestID:
		// The turn response is only an acknowledgement. Streaming notifications
		// carry all user-visible state and the terminal status.
	}
}

func (run *appServerRun) finish() error {
	if scanErr := run.scanner.Err(); scanErr != nil && run.ctx.Err() == nil && run.protocolErr == nil {
		run.protocolErr = fmt.Errorf("Codex app-server stdout: %w", scanErr)
	}
	if run.protocolErr != nil || !run.terminal {
		_ = run.stdin.Close()
		if run.cmd.Process != nil {
			_ = run.cmd.Process.Kill()
		}
	}
	waitErr := run.cmd.Wait()
	stderrText := <-run.stderrDone

	if errors.Is(run.ctx.Err(), context.Canceled) {
		return nil
	}
	if run.protocolErr != nil {
		return &agentruntime.ProcessError{Err: run.protocolErr, Stderr: stderrText}
	}
	if run.runFailed {
		return &agentruntime.ProcessError{Err: agent.ErrRunFailed, Stderr: stderrText}
	}
	if !run.terminal {
		if waitErr == nil {
			waitErr = errors.New("Codex app-server closed before the turn completed")
		}
		return &agentruntime.ProcessError{Err: waitErr, Stderr: stderrText}
	}
	return nil
}

func captureAppServerStderr(reader io.Reader, logID string, done chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 8192), 1<<20)
	var captured strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("codex[%s] stderr: %s", logID, line)
		if captured.Len() < 64<<10 {
			captured.WriteString(line)
			captured.WriteByte('\n')
		}
	}
	done <- captured.String()
}
