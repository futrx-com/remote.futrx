package devinharness

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

const (
	// acpCancelTimeout is the maximum time to wait for the session/prompt
	// response (with stopReason "cancelled") after sending session/cancel.
	acpCancelTimeout = 30 * time.Second
	// acpTerminalDrainTimeout is the grace period for late notifications
	// after the terminal session/prompt response arrives.
	acpTerminalDrainTimeout = 3 * time.Second
)

var errACPCancelTimeout = fmt.Errorf("ACP server did not confirm cancellation within %d seconds", int(acpCancelTimeout/time.Second))

// acpRun owns one `devin acp` process for one Remote turn.
type acpRun struct {
	ctx           context.Context
	req           agent.RunRequest
	providerLabel string
	process       *acpProcess

	emit           func(agent.Event)
	eventParser    *acpEventParser
	requestHandler *acpRequestHandler

	sessionID       string
	cancelRequested bool
	cancelSent      bool
	terminal        bool
	interrupted     bool
	runFailed       bool
	protocolErr     error
	terminalEvent   *agent.Event
}

// Run owns one Devin ACP process for one Remote turn. Provider adapters supply
// their normalized identity and label while retaining responsibility for their
// CLI configuration, environment, and project preparation.
//
// The cmd's context should use context.WithoutCancel so the process outlives
// request cancellation long enough for the harness to send session/cancel and
// receive the terminal stopReason.
func Run(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	providerLabel string,
	emit func(agent.Event),
) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	return newACPRun(ctx, cmd, req, providerLabel, emit).execute()
}

func newACPRun(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	providerLabel string,
	emit func(agent.Event),
) *acpRun {
	return &acpRun{
		ctx:           ctx,
		req:           req,
		providerLabel: providerLabel,
		emit:          emit,
		process:       newACPProcess(cmd, req.Provider, providerLabel, req.ConversationID),
	}
}

func (run *acpRun) execute() error {
	if err := run.start(); err != nil {
		return err
	}
	if err := run.handshake(); err != nil {
		run.process.abort()
		return err
	}
	run.consumeOutput()
	return run.finish()
}

func (run *acpRun) start() error {
	if err := run.process.start(); err != nil {
		return err
	}
	run.eventParser = newACPEventParser(run.req, run.providerLabel)
	run.requestHandler = newACPRequestHandler(run.req, run.emit, run.process.write)
	return nil
}

// handshake performs the synchronous initialize → session/new (or
// session/resume) exchange before the prompt is sent. Each step waits for its
// response on the stdout scanner.
//
// Devin's ACP server loads credentials from disk on startup (the credential
// policy log says "Will accept host credentials if provided, otherwise fall
// back to env vars and stored CLI credentials"), so the authenticate step is
// not needed when the host has already run `devin auth login`. The
// notifications/initialized notification is also skipped because Devin's ACP
// server does not implement it (returns "Method not found").
func (run *acpRun) handshake() error {
	// Step 1: initialize
	if err := run.process.write(buildInitialize()); err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}
	initResult, err := run.waitForResponse(acpInitializeRequestID)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := run.parseInitializeResult(initResult); err != nil {
		return err
	}

	// Step 2: session/new or session/resume
	if run.req.ResumeID != "" {
		if err := run.process.write(buildSessionResume(run.req)); err != nil {
			return fmt.Errorf("send session/resume: %w", err)
		}
	} else {
		if err := run.process.write(buildSessionNew(run.req)); err != nil {
			return fmt.Errorf("send session/new: %w", err)
		}
	}
	sessionResult, err := run.waitForResponse(acpSessionRequestID)
	if err != nil {
		if run.req.ResumeID != "" && isMissingSession(err.Error()) {
			return fmt.Errorf("%w: %s", agent.ErrSessionNotFound, err)
		}
		return fmt.Errorf("session: %w", err)
	}
	if err := run.parseSessionResult(sessionResult); err != nil {
		return err
	}

	// Step 3: session/prompt
	if err := run.process.write(buildSessionPrompt(run.sessionID, run.req.Prompt)); err != nil {
		return fmt.Errorf("send session/prompt: %w", err)
	}
	return nil
}

// waitForResponse reads stdout lines until a response with the matching request
// ID arrives. Notifications and server requests arriving during the wait are
// dispatched immediately. This is used during the synchronous handshake phase.
func (run *acpRun) waitForResponse(expectedID acpRequestID) (json.RawMessage, error) {
	for run.process.scanner.Scan() {
		line := append([]byte(nil), run.process.scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var envelope acpEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			log.Printf("%s[%s] acp parse during handshake: %v", run.req.Provider, run.req.ConversationID, err)
			continue
		}

		// Server-to-client request during handshake — handle it.
		if envelope.Method != "" && len(envelope.ID) > 0 {
			if err := run.requestHandler.Handle(envelope); err != nil {
				return nil, err
			}
			continue
		}
		// Notification during handshake — emit events.
		if envelope.Method != "" {
			for _, event := range run.eventParser.ParseNotification(envelope.Method, envelope.Params) {
				run.emit(event)
			}
			continue
		}
		// Response — check if it matches.
		responseID, ok := acpResponseID(envelope.ID)
		if !ok {
			continue
		}
		if responseID != expectedID {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("%s", envelope.Error.Message)
		}
		return envelope.Result, nil
	}
	if err := run.process.scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s ACP server stdout: %w", run.providerLabel, err)
	}
	return nil, fmt.Errorf("%s ACP server closed before responding to request %d", run.providerLabel, expectedID)
}

func (run *acpRun) parseInitializeResult(result json.RawMessage) error {
	var initResult acpInitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("decode initialize response: %w", err)
	}
	// The agent may respond with a lower protocol version (confirmed v1).
	// Accept whatever the agent returns.
	//
	// Devin's ACP server loads credentials from disk on startup, so the
	// authenticate step is not needed. We still verify authMethods is present
	// as a sanity check that the server initialized correctly.
	if len(initResult.AuthMethods) == 0 {
		return fmt.Errorf("%s ACP server returned no auth methods", run.providerLabel)
	}
	return nil
}

func (run *acpRun) parseSessionResult(result json.RawMessage) error {
	var sessionResult acpSessionResult
	if err := json.Unmarshal(result, &sessionResult); err != nil {
		return fmt.Errorf("decode session response: %w", err)
	}
	if sessionResult.SessionID == "" {
		return fmt.Errorf("%s ACP server returned no session id", run.providerLabel)
	}
	run.sessionID = sessionResult.SessionID
	if sessionResult.SessionID != run.req.ResumeID {
		run.emit(agent.Event{
			T:              time.Now().UnixMilli(),
			Type:           agent.EventSessionUpdated,
			Provider:       run.req.Provider,
			ConversationID: run.req.ConversationID,
			SessionID:      sessionResult.SessionID,
		})
	}
	return nil
}

// consumeOutput is the main event loop. It selects over stdout notifications,
// user interaction responses, context cancellation, and the terminal
// session/prompt response.
func (run *acpRun) consumeOutput() {
	scanned := make(chan acpScanResult, 1)
	stopScan := make(chan struct{})
	defer close(stopScan)
	go run.process.scan(scanned, stopScan)

	ctxDone := run.ctx.Done()
	responses := run.req.InteractionResponses
	var cancelTimer *time.Timer
	var cancelTimeout <-chan time.Time
	var terminalTimer *time.Timer
	var terminalTimeout <-chan time.Time
	defer func() {
		if cancelTimer != nil {
			cancelTimer.Stop()
		}
		if terminalTimer != nil {
			terminalTimer.Stop()
		}
	}()
	startTerminalDrain := func() {
		if !run.terminal || terminalTimer != nil {
			return
		}
		ctxDone = nil
		responses = nil
		cancelTimeout = nil
		terminalTimer = time.NewTimer(acpTerminalDrainTimeout)
		terminalTimeout = terminalTimer.C
	}

	for {
		select {
		case result, ok := <-scanned:
			if !ok {
				run.finalizeTerminal()
				return
			}
			if result.err != nil {
				if run.terminal {
					run.finalizeTerminal()
					return
				}
				run.protocolErr = result.err
				return
			}
			if run.terminal {
				run.handlePostTerminalEnvelope(result.envelope)
			} else if !run.handleEnvelope(result.envelope) {
				return
			}
			if run.cancelSent && cancelTimer == nil {
				cancelTimer = time.NewTimer(acpCancelTimeout)
				cancelTimeout = cancelTimer.C
			}
			startTerminalDrain()

		case response, ok := <-responses:
			if !ok {
				responses = nil
				continue
			}
			if err := run.requestHandler.Respond(response); err != nil {
				run.protocolErr = err
				return
			}

		case <-ctxDone:
			ctxDone = nil
			run.cancelRequested = true
			if err := run.maybeCancel(); err != nil {
				run.protocolErr = err
				return
			}
			if run.cancelSent && cancelTimer == nil {
				cancelTimer = time.NewTimer(acpCancelTimeout)
				cancelTimeout = cancelTimer.C
			}

		case <-cancelTimeout:
			run.protocolErr = errACPCancelTimeout
			return

		case <-terminalTimeout:
			run.finalizeTerminal()
			run.process.kill()
			return
		}
	}
}

func (run *acpRun) handlePostTerminalEnvelope(envelope acpEnvelope) {
	// Drain late notifications after the terminal prompt response.
	if envelope.Method != "" && len(envelope.ID) == 0 {
		run.handleNotification(envelope)
	}
}

func (run *acpRun) handleEnvelope(envelope acpEnvelope) bool {
	// Server-to-client request (method + id).
	if envelope.Method != "" && len(envelope.ID) > 0 {
		run.protocolErr = run.requestHandler.Handle(envelope)
		return run.protocolErr == nil
	}
	// Notification (method, no id).
	if envelope.Method != "" {
		run.handleNotification(envelope)
		return run.protocolErr == nil
	}
	// Response (id, no method) — must be the session/prompt response.
	responseID, ok := acpResponseID(envelope.ID)
	if !ok {
		return true
	}
	if envelope.Error != nil {
		run.protocolErr = fmt.Errorf("%s session/prompt: %s", run.providerLabel, envelope.Error.Message)
		return false
	}
	run.handlePromptResponse(responseID, envelope.Result)
	return run.protocolErr == nil
}

func (run *acpRun) handleNotification(envelope acpEnvelope) {
	if run.cancelRequested {
		if err := run.maybeCancel(); err != nil {
			run.protocolErr = err
			return
		}
	}
	for _, event := range run.eventParser.ParseNotification(envelope.Method, envelope.Params) {
		if isTerminalEvent(event.Type) {
			run.beginTerminal(event)
			continue
		}
		run.emit(event)
	}
}

func (run *acpRun) handlePromptResponse(responseID acpRequestID, resultJSON json.RawMessage) {
	if responseID != acpPromptRequestID {
		return
	}
	var result acpSessionPromptResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		run.protocolErr = fmt.Errorf("decode %s session/prompt response: %w", run.providerLabel, err)
		return
	}
	run.beginTerminal(run.promptResponseEvent(result))
}

func (run *acpRun) promptResponseEvent(result acpSessionPromptResult) agent.Event {
	now := time.Now().UnixMilli()
	switch result.StopReason {
	case StopReasonCancelled:
		run.interrupted = true
		return agent.Event{
			T:              now,
			Type:           agent.EventRunInterrupted,
			Provider:       run.req.Provider,
			ConversationID: run.req.ConversationID,
			Usage:          cloneRaw(result.Usage),
		}
	case StopReasonRefusal:
		run.runFailed = true
		return agent.Event{
			T:              now,
			Type:           agent.EventRunFailed,
			Provider:       run.req.Provider,
			ConversationID: run.req.ConversationID,
			Message:        "agent refused to continue",
			IsError:        true,
			Usage:          cloneRaw(result.Usage),
		}
	default:
		// end_turn, max_tokens, max_turn_requests → completed.
		return agent.Event{
			T:              now,
			Type:           agent.EventRunCompleted,
			Provider:       run.req.Provider,
			ConversationID: run.req.ConversationID,
			Usage:          cloneRaw(result.Usage),
		}
	}
}

func isTerminalEvent(eventType agent.EventType) bool {
	switch eventType {
	case agent.EventRunCompleted, agent.EventRunFailed, agent.EventRunInterrupted:
		return true
	default:
		return false
	}
}

func (run *acpRun) beginTerminal(event agent.Event) {
	if run.terminal {
		return
	}
	run.terminal = true
	run.runFailed = run.runFailed || event.Type == agent.EventRunFailed
	run.interrupted = run.interrupted || event.Type == agent.EventRunInterrupted
	run.terminalEvent = &event
	run.requestHandler.ResolveAll("turn_ended")
	run.process.closeInput()
}

func (run *acpRun) finalizeTerminal() {
	if run.terminalEvent == nil {
		return
	}
	run.emit(*run.terminalEvent)
	run.terminalEvent = nil
}

func (run *acpRun) maybeCancel() error {
	if !run.cancelRequested || run.cancelSent || run.sessionID == "" {
		return nil
	}
	if err := run.process.write(buildSessionCancel(run.sessionID)); err != nil {
		return fmt.Errorf("send session/cancel: %w", err)
	}
	run.cancelSent = true
	return nil
}

func (run *acpRun) finish() error {
	if run.terminal {
		run.finalizeTerminal()
	}
	if run.protocolErr != nil || !run.terminal {
		run.process.closeInput()
		run.process.kill()
	}
	waitErr, stderrText := run.process.wait()

	if run.protocolErr != nil {
		return &agentruntime.ProcessError{Err: run.protocolErr, Stderr: stderrText}
	}
	if run.runFailed {
		return &agentruntime.ProcessError{Err: agent.ErrRunFailed, Stderr: stderrText}
	}
	if run.interrupted {
		return nil
	}
	if !run.terminal {
		if waitErr == nil {
			waitErr = fmt.Errorf("%s ACP server closed before the turn completed", run.providerLabel)
		}
		return &agentruntime.ProcessError{Err: waitErr, Stderr: stderrText}
	}
	return nil
}

func acpResponseID(raw json.RawMessage) (acpRequestID, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var id acpRequestID
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, false
	}
	return id, true
}

func isMissingSession(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "not found") || strings.Contains(lower, "no session")
}
