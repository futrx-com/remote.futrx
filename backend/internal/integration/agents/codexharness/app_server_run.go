package codexharness

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

var errAppServerInterruptTimeout = fmt.Errorf(
	"app-server did not complete turn/interrupt within %d seconds",
	configconstants.CodexHarnessInterruptTimeout/time.Second,
)

type appServerRequestID int

const (
	appServerInitializeRequestID appServerRequestID = iota + 1
	appServerThreadRequestID
	appServerTurnRequestID
	appServerInterruptRequestID
)

type appServerRun struct {
	ctx           context.Context
	req           agent.RunRequest
	providerLabel string
	process       *appServerProcess

	emit               func(agent.Event)
	eventParser        *appServerEventParser
	subagents          *appServerSubagentTracker
	requestHandler     *appServerRequestHandler
	threadID           string
	turnID             string
	cancelRequested    bool
	interruptSent      bool
	terminal           bool
	interrupted        bool
	runFailed          bool
	protocolErr        error
	terminalEvent      *agent.Event
	openCollaborations map[string]agent.Event
}

// Run owns one Codex app-server process for one Remote turn. Provider adapters
// supply their normalized identity and label while retaining responsibility
// for their CLI configuration, environment, and project preparation.
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
	return newAppServerRun(ctx, cmd, req, providerLabel, emit).execute()
}

func newAppServerRun(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	providerLabel string,
	emit func(agent.Event),
) *appServerRun {
	return &appServerRun{
		ctx:                ctx,
		req:                req,
		providerLabel:      providerLabel,
		emit:               emit,
		process:            newAppServerProcess(cmd, req.Provider, providerLabel, req.ConversationID),
		openCollaborations: make(map[string]agent.Event),
	}
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
	if err := run.process.start(); err != nil {
		return err
	}
	run.eventParser = newAppServerEventParser(run.req, run.providerLabel)
	run.subagents = newAppServerSubagentTracker(run.eventParser)
	run.requestHandler = newAppServerRequestHandler(run.req, run.emit, run.process.write)
	return nil
}

func (run *appServerRun) initialize() error {
	return run.process.write(map[string]any{
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
	run.process.abort()
}

func (run *appServerRun) consumeOutput() {
	scanned := make(chan appServerScanResult, 1)
	stopScan := make(chan struct{})
	defer close(stopScan)
	go run.process.scan(scanned, stopScan)

	ctxDone := run.ctx.Done()
	responses := run.req.InteractionResponses
	var interruptTimer *time.Timer
	var interruptTimeout <-chan time.Time
	var terminalTimer *time.Timer
	var terminalTimeout <-chan time.Time
	defer func() {
		if interruptTimer != nil {
			interruptTimer.Stop()
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
		interruptTimeout = nil
		terminalTimer = time.NewTimer(configconstants.CodexHarnessTerminalDrainTimeout)
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
			if run.interruptSent && interruptTimer == nil {
				interruptTimer = time.NewTimer(configconstants.CodexHarnessInterruptTimeout)
				interruptTimeout = interruptTimer.C
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
			if err := run.maybeInterrupt(); err != nil {
				run.protocolErr = err
				return
			}
			if run.interruptSent && interruptTimer == nil {
				interruptTimer = time.NewTimer(configconstants.CodexHarnessInterruptTimeout)
				interruptTimeout = interruptTimer.C
			}

		case <-interruptTimeout:
			run.protocolErr = errAppServerInterruptTimeout
			return

		case <-terminalTimeout:
			run.finalizeTerminal()
			run.process.kill()
			return
		}
	}
}

func (run *appServerRun) handlePostTerminalEnvelope(envelope appServerEnvelope) {
	// A provider may flush item lifecycle notifications immediately after the
	// authoritative turn/completed notification. Only drain notifications here:
	// the turn has ended, so no new requests or responses should be acted on.
	if envelope.Method != "" && len(envelope.ID) == 0 {
		run.handleNotification(envelope)
	}
}

func (run *appServerRun) handleEnvelope(envelope appServerEnvelope) bool {
	if envelope.Method != "" && len(envelope.ID) > 0 {
		ids := nativeIDs(envelope.Params)
		if run.threadID == "" {
			run.threadID = ids.ThreadID
		}
		if run.turnID == "" {
			run.turnID = ids.TurnID
		}
		run.protocolErr = run.requestHandler.Handle(envelope)
		if run.protocolErr == nil && run.cancelRequested {
			run.protocolErr = run.maybeInterrupt()
		}
		return run.protocolErr == nil
	}
	if envelope.Method != "" {
		if envelope.Method == "serverRequest/resolved" {
			if event := run.requestHandler.Resolve(envelope.Params); event != nil {
				run.emit(*event)
			}
		}
		run.handleNotification(envelope)
		return run.protocolErr == nil
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
	ids := nativeIDs(envelope.Params)
	if run.threadID == "" {
		run.threadID = ids.ThreadID
	}
	if run.threadID != "" && ids.ThreadID != "" && ids.ThreadID != run.threadID {
		// App Server multiplexes descendant subagent threads on the same stdout
		// stream. Keep their activity inspectable without allowing their messages,
		// usage, or turn completion to become the parent run's events.
		for _, event := range run.subagents.ParseNotification(run.threadID, envelope.Method, envelope.Params) {
			run.emit(event)
		}
		return
	}
	if run.turnID == "" && (ids.ThreadID == "" || ids.ThreadID == run.threadID) {
		run.turnID = ids.TurnID
	}
	if run.cancelRequested {
		if err := run.maybeInterrupt(); err != nil {
			run.protocolErr = err
			return
		}
	}

	for _, event := range run.eventParser.ParseNotification(envelope.Method, envelope.Params) {
		if isTerminalEvent(event.Type) {
			run.beginTerminal(event)
			continue
		}
		run.trackCollaboration(event)
		run.emit(event)
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

func (run *appServerRun) beginTerminal(event agent.Event) {
	if run.terminal {
		return
	}
	run.terminal = true
	run.runFailed = event.Type == agent.EventRunFailed
	run.interrupted = event.Type == agent.EventRunInterrupted
	run.terminalEvent = &event
	run.requestHandler.ResolveAll("turn_ended")
	run.process.closeInput()
}

func (run *appServerRun) trackCollaboration(event agent.Event) {
	if event.Type != agent.EventCollaboration || event.ItemID == "" {
		return
	}
	if event.Native != nil && event.Native.Method == "item/completed" {
		delete(run.openCollaborations, event.ItemID)
		return
	}
	run.openCollaborations[event.ItemID] = event
}

func (run *appServerRun) finalizeTerminal() {
	if run.terminalEvent == nil {
		return
	}
	rootStatus := "completed"
	collaborationStatus := "turnEnded"
	if run.interrupted {
		rootStatus = "interrupted"
		collaborationStatus = "interrupted"
	} else if run.runFailed {
		rootStatus = "failed"
		collaborationStatus = "failed"
	}
	for _, event := range run.subagents.Finalize(rootStatus) {
		run.emit(event)
	}
	run.finalizeOpenCollaborations(collaborationStatus)
	run.emit(*run.terminalEvent)
	run.terminalEvent = nil
}

func (run *appServerRun) finalizeOpenCollaborations(status string) {
	if len(run.openCollaborations) == 0 {
		return
	}
	ids := make([]string, 0, len(run.openCollaborations))
	for id := range run.openCollaborations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		event := run.openCollaborations[id]
		event.T = time.Now().UnixMilli()
		event.Status = status
		event.Data = collaborationResolutionData(event.Data, status)
		event.Raw = nil
		event.Native = nil
		run.emit(event)
		delete(run.openCollaborations, id)
	}
}

func collaborationResolutionData(raw json.RawMessage, status string) json.RawMessage {
	data := make(map[string]any)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	data["status"] = status
	data["remoteResolution"] = "missingItemCompletion"
	encoded, err := json.Marshal(data)
	if err != nil {
		return cloneRaw(raw)
	}
	return encoded
}

func (run *appServerRun) maybeInterrupt() error {
	if !run.cancelRequested || run.interruptSent || run.threadID == "" || run.turnID == "" {
		return nil
	}
	if err := run.process.write(map[string]any{
		"method": "turn/interrupt",
		"id":     appServerInterruptRequestID,
		"params": map[string]string{
			"threadId": run.threadID,
			"turnId":   run.turnID,
		},
	}); err != nil {
		return err
	}
	run.interruptSent = true
	return nil
}

func (run *appServerRun) responseError(responseID appServerRequestID, message string) error {
	message = strings.TrimSpace(message)
	if responseID == appServerThreadRequestID && run.req.ResumeID != "" && isMissingThread(message) {
		return fmt.Errorf("%w: %s", agent.ErrSessionNotFound, message)
	}
	return fmt.Errorf("%s app-server request %d: %s", run.providerLabel, responseID, message)
}

func (run *appServerRun) handleResponse(responseID appServerRequestID, resultJSON json.RawMessage) {
	switch responseID {
	case appServerInitializeRequestID:
		if err := run.process.write(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
			run.protocolErr = err
			return
		}
		request := buildAppServerThreadRequest(run.req)
		run.protocolErr = run.process.write(map[string]any{
			"method": request.Method,
			"id":     appServerThreadRequestID,
			"params": request.Params,
		})

	case appServerThreadRequestID:
		var result appServerThreadResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			run.protocolErr = fmt.Errorf("decode %s thread response: %w", run.providerLabel, err)
			return
		}
		if result.Thread.ID == "" || result.Model == "" {
			run.protocolErr = fmt.Errorf("%s app-server returned an incomplete thread", run.providerLabel)
			return
		}
		run.threadID = result.Thread.ID
		// The server resolves aliases such as "auto" to the concrete model.
		// Carry that model into the completion usage persisted for rebuilds.
		run.eventParser.req.Model = result.Model
		if result.Thread.ID != run.req.ResumeID {
			run.emit(agent.Event{
				T:              time.Now().UnixMilli(),
				Type:           agent.EventSessionUpdated,
				Provider:       run.req.Provider,
				ConversationID: run.req.ConversationID,
				SessionID:      result.Thread.ID,
				Model:          result.Model,
			})
		}
		run.protocolErr = run.process.write(map[string]any{
			"method": "turn/start",
			"id":     appServerTurnRequestID,
			"params": buildAppServerTurnParams(run.req, result.Thread.ID, result.Model),
		})

	case appServerTurnRequestID:
		var result struct {
			Turn appServerTurnResult `json:"turn"`
		}
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			run.protocolErr = fmt.Errorf("decode %s turn response: %w", run.providerLabel, err)
			return
		}
		if result.Turn.ID == "" {
			run.protocolErr = fmt.Errorf("%s app-server returned a turn without an id", run.providerLabel)
			return
		}
		run.turnID = result.Turn.ID
		if run.cancelRequested {
			run.protocolErr = run.maybeInterrupt()
		}

	case appServerInterruptRequestID:
		// The acknowledgement is not terminal. Keep consuming notifications until
		// turn/completed reports the authoritative interrupted status.
	}
}

func rpcResponseID(raw json.RawMessage) (appServerRequestID, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var id appServerRequestID
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, false
	}
	return id, true
}

func isMissingThread(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "not found") || strings.Contains(lower, "no rollout")
}

func (run *appServerRun) finish() error {
	if run.terminal {
		run.finalizeTerminal()
	} else {
		for _, event := range run.subagents.Finalize("failed") {
			run.emit(event)
		}
		run.finalizeOpenCollaborations("failed")
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
			waitErr = fmt.Errorf("%s app-server closed before the turn completed", run.providerLabel)
		}
		return &agentruntime.ProcessError{Err: waitErr, Stderr: stderrText}
	}
	return nil
}
