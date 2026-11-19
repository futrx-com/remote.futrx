package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
)

type ChatEvent = servicechat.Event
type ChatMeta = servicechat.Meta

type TmuxClient interface {
	Cwd(session string) (string, error)
}

// ProjectResolver decouples runner from project service internals. Lets tests
// stub project lookup/start without pulling in HTTP or persistence.
type ProjectResolver interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	Start(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	ListSecrets(ctx context.Context, id serviceproject.ID) ([]serviceproject.Secret, error)
}

type agentBrowserActivityRecorder interface {
	TouchAgentBrowserActivity(ctx context.Context, id serviceproject.ID)
}

var ErrPromptAlreadyRunning = errors.New("a previous prompt is still running")
var ErrUnsupportedAgentScope = errors.New("agent does not support this chat execution scope")
var ErrMaintenance = errors.New("an infrastructure update is recycling workspaces; prompts will be available when it finishes")

type Actor struct {
	Email   string
	IsAdmin bool
}

type StartInput struct {
	ChatID                servicechat.ID
	Prompt                string
	Actor                 Actor
	ApplicationInstanceID string
	ApplicationID         string
	ApplicationRequestID  string
	ApplicationContext    json.RawMessage
	ParentContext         context.Context
}

type RunResult struct {
	Output string
	Err    error
}

type RunHandle struct {
	ID     uint64
	TurnID string
	Done   <-chan RunResult
}

type ApplicationToolRequest struct {
	Actor                 Actor
	ChatID                servicechat.ID
	ProjectID             serviceproject.ID
	ApplicationInstanceID string
	ApplicationRequestID  string
	Context               json.RawMessage
}
type ApplicationToolAccess struct {
	Env    map[string]string
	Skills []servicechat.SkillRef
	Revoke func()
}
type ApplicationToolIssuer interface {
	IssueApplicationTools(context.Context, ApplicationToolRequest) (ApplicationToolAccess, error)
}

// UsageRecorder receives one entry per completed agent run. It is the only
// thing the prompt service knows about token accounting; pricing, storage and
// aggregation all live in the usage service.
type UsageRecorder interface {
	RecordRun(ctx context.Context, event serviceusage.RunEvent)
}

// QuotaRecorder files the subscription windows the agent CLIs volunteer. A
// plan belongs to the provider account that ran, so each window arrives with
// the saved account ID its adapter stamped, or an empty ID for the provider's
// host login. It is optional: without one the readings are dropped and the
// dashboard has no plan card, which is the behaviour before this existed.
type QuotaRecorder interface {
	Record(ctx context.Context, provider harness.ProviderID, accountID string, quota harness.Quota)
}

type Option func(*Service)

// StartGate blocks new agent runs while an external host job owns the
// workspace lifecycle. Existing runs remain untouched and are skipped by the
// workspace upgrader.
type StartGate interface {
	Blocked() bool
}

func WithStartGate(gate StartGate) Option {
	return func(service *Service) {
		service.startGate = gate
	}
}

func (s *Service) SetApplicationToolIssuer(issuer ApplicationToolIssuer) { s.applicationTools = issuer }

func WithApplicationToolIssuer(issuer ApplicationToolIssuer) Option {
	return func(service *Service) {
		service.applicationTools = issuer
	}
}

func WithUsageRecorder(recorder UsageRecorder) Option {
	return func(service *Service) {
		service.usage = recorder
	}
}

// WithQuotaRecorder installs the recipient of provider quota observations.
func WithQuotaRecorder(recorder QuotaRecorder) Option {
	return func(service *Service) {
		service.quota = recorder
	}
}

type AgentPolicy interface {
	Descriptor(provider string) (agentmodule.Descriptor, bool)
	SupportsScope(provider string, scope agentmodule.ExecutionScope) bool
}

type AgentRegistry interface {
	Lookup(harness.ProviderID) harness.Provider
}

func WithAgentPolicy(policy AgentPolicy) Option {
	return func(service *Service) {
		service.agentPolicy = policy
	}
}

func WithConversationRuntime(runtime servicechat.ConversationRuntime) Option {
	return func(service *Service) { service.conversationRuntime = runtime }
}

type Service struct {
	conversationRuntime servicechat.ConversationRuntime
	accountAccess       AccountAccess
	store               servicechat.Repository
	tmux                TmuxClient
	projects            ProjectResolver
	hub                 *runhub.Hub
	agents              AgentRegistry
	agentPolicy         AgentPolicy
	applicationTools    ApplicationToolIssuer
	usage               UsageRecorder
	quota               QuotaRecorder
	startGate           StartGate
	interactions        interactionResponseRouter
	projectRuns         projectAdmissions
	audit               audit.Recorder
}

func New(
	store servicechat.Repository,
	tmux TmuxClient,
	projects ProjectResolver,
	hub *runhub.Hub,
	agents AgentRegistry,
	options ...Option,
) *Service {
	if hub == nil {
		hub = runhub.New(store)
	}
	service := &Service{
		store:        store,
		tmux:         tmux,
		projects:     projects,
		hub:          hub,
		agents:       agents,
		interactions: newInteractionResponseRouter(),
		audit:        audit.Nop{},
	}
	service.conversationRuntime, _ = store.(servicechat.ConversationRuntime)
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (rnr *Service) StartPrompt(id servicechat.ID, prompt string, emitTransient func(ChatEvent)) {
	_, _ = rnr.Start(StartInput{ChatID: id, Prompt: prompt}, emitTransient)
}

func (rnr *Service) Start(input StartInput, emitTransient func(ChatEvent)) (RunHandle, error) {
	if emitTransient == nil {
		emitTransient = func(ChatEvent) {}
	}
	if rnr.startGate != nil && rnr.startGate.Blocked() {
		emitTransient(ChatEvent{
			T: time.Now().UnixMilli(), Type: "error", Message: ErrMaintenance.Error(),
		})
		return RunHandle{}, ErrMaintenance
	}
	parentCtx := input.ParentContext
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	if rnr.accountAccess != nil {
		parentCtx = accountActorContext(parentCtx, input)
		meta, err := rnr.store.Get(parentCtx, input.ChatID)
		if err == nil {
			_, err = rnr.accountAccess.Resolve(parentCtx, harness.ProviderID(meta.Provider), meta.AccountID)
		}
		if err != nil {
			emitTransient(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
			return RunHandle{}, err
		}
	}
	// Account authorization has no side effects, so it runs before admission
	// reserves the project.
	release, err := rnr.admitProject(parentCtx, input)
	if err != nil {
		emitTransient(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
		return RunHandle{}, err
	}
	ctx, cancel := context.WithCancel(parentCtx)
	runID, ok := rnr.hub.StartRun(input.ChatID, cancel)
	if !ok {
		release()
		cancel()
		rnr.recordRun(parentCtx, audit.ActionAgentRunStart, input, ErrPromptAlreadyRunning)
		emitTransient(ChatEvent{
			T: time.Now().UnixMilli(), Type: "error",
			Message: "a previous prompt is still running — cancel first",
		})
		return RunHandle{}, ErrPromptAlreadyRunning
	}
	responses := rnr.interactions.open(input.ChatID, runID)

	done := make(chan RunResult, 1)
	ledgerRunID := newLedgerRunID()
	go func() {
		defer cancel()
		defer release()
		defer close(done)
		defer rnr.hub.FinishRun(input.ChatID, runID)
		defer rnr.interactions.remove(input.ChatID, runID)
		var output strings.Builder
		err := rnr.runPromptAs(
			ctx,
			input,
			ledgerRunID,
			responses,
			func(ev ChatEvent) {
				// Preserve the application origin without interpreting its workflow.
				ev.ApplicationID = input.ApplicationID
				ev.ApplicationRequestID = input.ApplicationRequestID
				rnr.hub.Emit(input.ChatID, ev)
				if ev.Type == "assistant_text" {
					output.WriteString(ev.Text)
				}
			},
			emitTransient,
		)
		done <- RunResult{Output: output.String(), Err: err}
	}()
	return RunHandle{ID: runID, TurnID: ledgerRunID, Done: done}, nil
}

func (rnr *Service) CancelPrompt(ctx context.Context, id servicechat.ID) bool {
	canceled := rnr.hub.CancelRun(id)
	rnr.recordRunCancellation(ctx, id, canceled)
	return canceled
}

func (rnr *Service) RespondInteraction(id servicechat.ID, response harness.InteractionResponse) error {
	return rnr.interactions.respond(id, response)
}

func (rnr *Service) runPrompt(
	ctx context.Context,
	id servicechat.ID,
	prompt string,
	emit func(ChatEvent),
	emitTransient func(ChatEvent),
) error {
	return rnr.runPromptAs(
		ctx,
		StartInput{ChatID: id, Prompt: prompt},
		newLedgerRunID(),
		nil,
		emit,
		emitTransient,
	)
}

func (rnr *Service) runPromptAs(
	ctx context.Context,
	input StartInput,
	ledgerRunID string,
	interactionResponses <-chan harness.InteractionResponse,
	emit func(ChatEvent),
	emitTransient func(ChatEvent),
) (resultErr error) {
	emit = withTurnID(ledgerRunID, emit)
	emitUnattributed := emit
	emit = func(event ChatEvent) {
		// The actor comes from the authenticated transport or captured application
		// owner, never from provider output or the client's prompt payload.
		event.UserEmail = input.Actor.Email
		emitUnattributed(event)
	}
	id := input.ChatID
	prompt := input.Prompt
	meta, err := rnr.store.Get(ctx, id)
	if err != nil {
		emitTransient(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
		return err
	}

	if rnr.accountAccess != nil {
		ctx = accountActorContext(ctx, input)
		selected, err := rnr.accountAccess.Resolve(ctx, harness.ProviderID(meta.Provider), meta.AccountID)
		if err != nil {
			emitTransient(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
			return err
		}
		meta.AccountID = selected
		ctx = audit.WithProject(ctx, string(meta.ProjectID))
	}

	ctx = audit.WithProject(audit.EnsureCaller(ctx, audit.Caller{Actor: audit.Actor{Email: input.Actor.Email, IsAdmin: input.Actor.IsAdmin}}), string(meta.ProjectID))
	if rnr.audit != nil {
		entry := rnr.recordStartedRun(ctx, input, meta, ledgerRunID)
		defer func() { rnr.recordCompletedRun(ctx, entry, resultErr) }()
	}

	// Auto-title from first user prompt if still default.
	if meta.Title == "" || meta.Title == "New chat" {
		_, _ = rnr.store.Update(ctx, id, func(m *ChatMeta) {
			m.Title = servicechat.TitleFromPrompt(prompt)
		})
	}

	// Project metadata stores the host-side bind-mount source, but provider
	// processes run inside the project container where that workspace is always
	// mounted at /workspace. Never pass the host path into an in-container CLI:
	// Codex-harness tools and MCP servers use this request cwd when they spawn.
	cwd := meta.Cwd
	if meta.ProjectID != "" {
		cwd = harness.ProjectWorkspacePath
	} else if meta.TmuxSession != "" {
		if c, err := rnr.tmux.Cwd(meta.TmuxSession); err == nil && c != "" {
			cwd = c
		}
	}
	if cwd == "" {
		cwd = os.Getenv("HOME")
		if cwd == "" {
			cwd = "/root"
		}
	}

	providerID := providerIDFromChatProvider(meta.Provider)
	descriptor := agentmodule.Descriptor{}
	if rnr.agentPolicy != nil {
		descriptor, _ = rnr.agentPolicy.Descriptor(string(providerID))
		scope := agentmodule.ScopeHost
		if meta.ProjectID != "" {
			scope = agentmodule.ScopeProject
		}
		if !rnr.agentPolicy.SupportsScope(string(providerID), scope) {
			emitTransient(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: ErrUnsupportedAgentScope.Error()})
			return ErrUnsupportedAgentScope
		}
	}
	promptSkills := append([]servicechat.SkillRef(nil), meta.SelectedSkills...)
	runtimeEnv := map[string]string(nil)
	if descriptor.Features.ApplicationTools && rnr.applicationTools != nil {
		access, err := rnr.applicationTools.IssueApplicationTools(ctx, ApplicationToolRequest{
			Actor: input.Actor, ChatID: id, ProjectID: serviceproject.ID(meta.ProjectID),
			ApplicationInstanceID: input.ApplicationInstanceID, ApplicationRequestID: input.ApplicationRequestID,
			Context: input.ApplicationContext,
		})
		if err != nil {
			return err
		}
		if access.Revoke != nil {
			defer access.Revoke()
		}
		runtimeEnv = access.Env
		for _, skill := range access.Skills {
			found := false
			for _, selected := range promptSkills {
				if skillTriggerName(selected.Command) == skillTriggerName(skill.Command) {
					found = true
					break
				}
			}
			if !found {
				skill.Provider = servicechat.Provider(providerID)
				promptSkills = append(promptSkills, skill)
			}
		}
	}
	resumeID := sessionIDForProvider(meta, providerID)
	if rnr.agentPolicy != nil && !descriptor.Features.Sessions.Resume {
		resumeID = ""
	}
	forkSession := meta.ForkPending
	if rnr.agentPolicy != nil && !descriptor.Features.Sessions.Fork {
		forkSession = false
	}
	effectivePrompt := prompt
	enableBrowser := descriptor.Features.BrowserTools && hasBrowserSkill(meta.SelectedSkills)
	if enableBrowser && meta.ProjectID != "" {
		stopBrowserKeepalive := rnr.keepAgentBrowserActivity(ctx, serviceproject.ID(meta.ProjectID))
		defer stopBrowserKeepalive()
	}
	effectivePrompt = promptWithSelectedSkills(
		descriptor.Features.Skills,
		descriptor.Label,
		providerID,
		promptSkills,
		meta.ProjectID != "",
		effectivePrompt,
	)

	provider := rnr.agents.Lookup(providerID)
	if provider == nil {
		err := errors.New(string(providerID) + " provider not configured")
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
		return err
	}

	var priorEvents []ChatEvent
	var prepared servicechat.PreparedRun
	if rnr.conversationRuntime != nil {
		prepared, err = rnr.conversationRuntime.PrepareRun(ctx, servicechat.RunPreparation{
			ChatID: id, TurnID: ledgerRunID, RunID: ledgerRunID, Provider: providerID,
			AccountID: meta.AccountID, Model: meta.Model,
			ForceFresh: forkSession || (rnr.agentPolicy != nil && !descriptor.Features.Sessions.Resume),
		})
		if err != nil {
			return err
		}
		resumeID = prepared.ResumeID
		// Branches have their own native session mappings. Native fork would
		// append/fork unrelated state after a history-based branch was created.
		forkSession = false
	} else {
		// Compatibility for non-SQLite repositories used by embedding callers.
		priorEvents, err = rnr.store.ReadEvents(ctx, id)
		if err != nil {
			return err
		}
		if resumeID == "" {
			effectivePrompt = promptWithVisibleHistory(priorEvents, effectivePrompt)
		}
	}

	// The handoff range was captured before this append, so the current user
	// request is delivered exactly once and never inserted into history twice.
	emit(ChatEvent{T: time.Now().UnixMilli(), Type: "user", Text: prompt, Provider: providerID, AccountID: meta.AccountID})

	ledger := ledgerRun{
		runID:                ledgerRunID,
		chatID:               id,
		projectID:            string(meta.ProjectID),
		userEmail:            input.Actor.Email,
		model:                meta.Model,
		applicationID:        input.ApplicationID,
		applicationRequestID: input.ApplicationRequestID,
	}

	run := func(runPrompt, runResumeID string) (runErr error) {
		var tracking *runDelivery
		if rnr.conversationRuntime != nil {
			tracking = &runDelivery{store: rnr.conversationRuntime, runID: prepared.RunID, nativeID: runResumeID}
			defer func() {
				if persistenceErr := rnr.hub.PersistenceError(id); persistenceErr != nil {
					runErr = errors.Join(runErr, persistenceErr)
				}
				runErr = tracking.finish(context.WithoutCancel(ctx), ctx.Err(), runErr)
			}()
		}
		if persistenceErr := rnr.hub.PersistenceError(id); persistenceErr != nil {
			return persistenceErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if rnr.accountAccess != nil {
			if _, err := rnr.accountAccess.Resolve(ctx, providerID, meta.AccountID); err != nil {
				return err
			}
		}
		rnr.recordAccountUse(ctx, input, meta, providerID, ledgerRunID)
		relay := runEventRelay{
			service: rnr, ctx: ctx, chatID: id, providerID: providerID,
			ledger: ledger, emit: emit,
		}
		if rnr.conversationRuntime != nil {
			adapter, ok := provider.(harness.HistoryInputAdapter)
			if !ok {
				return errors.New(string(providerID) + " does not implement conversation history delivery")
			}
			var method string
			var err error
			runPrompt, method, err = adapter.HistoryInput(prepared.History, runPrompt)
			if err != nil {
				return err
			}
			if err = rnr.conversationRuntime.RecordRunInput(ctx, prepared.RunID, runPrompt, method); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		runCtx, cancelRun := context.WithCancel(ctx)
		defer cancelRun()
		var completion *harness.Event
		runErr = provider.Run(runCtx, harness.RunRequest{
			TrackInputAcceptance: tracking != nil,
			Provider:             providerID,
			AccountID:            meta.AccountID,
			ConversationID:       string(id),
			Prompt:               runPrompt,
			Cwd:                  cwd,
			Model:                meta.Model,
			Mode:                 harness.RunMode(meta.Mode),
			ResumeID:             runResumeID,
			ProjectID:            string(meta.ProjectID),
			Fork:                 forkSession,
			Preferences: harness.RunPreferences{
				ReasoningEffort: harness.ReasoningEffort(meta.ReasoningEffort),
				ServiceTier:     harness.ServiceTier(meta.ServiceTier),
				ApprovalPolicy:  servicechat.NormalizeApprovalPolicy(meta.ApprovalPolicy),
				SandboxPolicy:   servicechat.NormalizeSandboxPolicy(meta.SandboxPolicy),
			},
			EnableBrowser:        enableBrowser,
			RuntimeEnv:           runtimeEnv,
			InteractionResponses: interactionResponses,
		}, func(event harness.Event) {
			event = withDefaultProvider(event, providerID)
			if event.AccountID == "" {
				event.AccountID = meta.AccountID
			}
			if tracking != nil {
				tracking.observe(context.WithoutCancel(ctx), event)
				if tracking.acceptanceErr != nil {
					cancelRun()
				}
			}
			if meta.SetupRepair && event.Type == harness.EventRunCompleted {
				completion = &event
				return
			}
			relay.forward(event)
		})
		if runErr == nil && meta.SetupRepair {
			verifier, ok := rnr.projects.(interface {
				VerifySetup(context.Context, serviceproject.ID) (serviceproject.Meta, error)
			})
			if !ok {
				runErr = errors.New("project setup verifier unavailable")
			} else {
				_, runErr = verifier.VerifySetup(ctx, serviceproject.ID(meta.ProjectID))
			}
		}
		if completion != nil {
			if runErr != nil {
				// The provider spent these tokens even when setup still fails.
				// Keep its completion for accounting, with a failed turn status
				// and a platform result instead of the agent's success summary.
				completion.Status = "failed"
				relay.emit = func(event ChatEvent) {
					if event.Type == "complete" {
						event.NotificationSummary = "Automatic setup repair did not pass verification. Open the repair chat for details."
					}
					emit(event)
				}
			}
			relay.forward(*completion)
		}
		relay.finish()
		return runErr
	}

	err = run(effectivePrompt, resumeID)
	if errors.Is(err, harness.ErrSessionNotFound) && resumeID != "" {
		_, _ = rnr.store.Update(ctx, id, func(m *ChatMeta) {
			clearSessionIDForProvider(m, providerID)
			m.ForkPending = false
		})
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "system", Subtype: "session_recovered"})
		freshPrompt := prompt
		if rnr.conversationRuntime != nil {
			through := prepared.History.ThroughSeq
			prepared, err = rnr.conversationRuntime.PrepareRun(ctx, servicechat.RunPreparation{
				ChatID: id, TurnID: ledgerRunID, RunID: newLedgerRunID(), Provider: providerID,
				AccountID: meta.AccountID, Model: meta.Model, ForceFresh: true, ThroughSeq: &through,
			})
			if err != nil {
				return err
			}
		} else {
			freshPrompt = promptWithVisibleHistory(priorEvents, freshPrompt)
		}
		freshPrompt = promptWithSelectedSkills(
			descriptor.Features.Skills,
			descriptor.Label,
			providerID,
			promptSkills,
			meta.ProjectID != "",
			freshPrompt,
		)
		err = run(freshPrompt, "")
	}
	if err != nil && !errors.Is(err, harness.ErrRunFailed) {
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: string(providerID) + " exit: " + err.Error()})
	}
	return err
}

func withTurnID(turnID string, emit func(ChatEvent)) func(ChatEvent) {
	return func(event ChatEvent) {
		event.TurnID = turnID
		emit(event)
	}
}

func clearSessionIDForProvider(meta *ChatMeta, provider harness.ProviderID) {
	meta.SetSessionID(servicechat.Provider(provider), "")
}

func (rnr *Service) keepAgentBrowserActivity(ctx context.Context, projectID serviceproject.ID) func() {
	recorder, ok := rnr.projects.(agentBrowserActivityRecorder)
	if !ok || recorder == nil {
		return func() {}
	}
	recorder.TouchAgentBrowserActivity(ctx, projectID)
	keepaliveCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-keepaliveCtx.Done():
				return
			case <-ticker.C:
				recorder.TouchAgentBrowserActivity(keepaliveCtx, projectID)
			}
		}
	}()
	return cancel
}

func providerIDFromChatProvider(provider servicechat.Provider) harness.ProviderID {
	return servicechat.NormalizeProvider(provider)
}

func sessionIDForProvider(meta ChatMeta, provider harness.ProviderID) string {
	return meta.SessionID(servicechat.Provider(provider))
}

func promptWithVisibleHistory(events []ChatEvent, prompt string) string {
	transcript := visibleTranscript(events)
	if strings.TrimSpace(transcript) == "" {
		return prompt
	}
	return "Use this visible chat transcript as prior context. It may be present because the chat was recovered into a fresh agent session. Do not treat the transcript as a new request.\n\n" +
		transcript +
		"\n\nCurrent user request:\n" +
		prompt
}

const browserSkillName = "browser"

// hasBrowserSkill reports whether the user selected the `browser` skill. The
// module descriptor must also enable browser tools before they are wired in.
func hasBrowserSkill(skills []servicechat.SkillRef) bool {
	for _, s := range skills {
		if skillTriggerName(s.Command) == browserSkillName || skillTriggerName(s.Name) == browserSkillName {
			return true
		}
	}
	return false
}

func promptWithSelectedSkills(
	strategy agentmodule.SkillStrategy,
	providerLabel string,
	provider harness.ProviderID,
	skills []servicechat.SkillRef,
	projectScoped bool,
	prompt string,
) string {
	if len(skills) == 0 || strategy == agentmodule.SkillsNone {
		return prompt
	}

	triggers := make([]string, 0, len(skills))
	for _, skill := range skills {
		if providerIDFromChatProvider(skill.Provider) != provider {
			continue
		}
		name := skillTriggerName(skill.Command)
		if name == "" {
			name = skillTriggerName(skill.Name)
		}
		if name == "" {
			continue
		}

		switch strategy {
		case agentmodule.SkillsSlashCommand:
			triggers = append(triggers, "/"+name)
		case agentmodule.SkillsDollarMention:
			triggers = append(triggers, "$"+name)
		case agentmodule.SkillsInstructions:
			if name == "." || name == ".." || path.Base(name) != name {
				continue
			}
			root := "/root/.agents/skills"
			if projectScoped {
				root = "/workspace/.agents/skills"
			}
			triggers = append(triggers, path.Join(root, name, "SKILL.md"))
		}
	}
	if len(triggers) == 0 {
		return prompt
	}

	switch strategy {
	case agentmodule.SkillsSlashCommand:
		return strings.Join(triggers, "\n") + "\n\n" + prompt
	case agentmodule.SkillsDollarMention:
		if strings.TrimSpace(providerLabel) == "" {
			providerLabel = string(provider)
		}
		return "Use these " + providerLabel + " skills for this request: " + strings.Join(triggers, " ") + "\n\n" + prompt
	case agentmodule.SkillsInstructions:
		return "Read and follow the selected skill instructions at " +
			strings.Join(triggers, ", ") + ".\n\n" + prompt
	default:
		return prompt
	}
}

func skillTriggerName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, "/$")
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	if len(parts) <= 1 {
		return value
	}
	return strings.Join(parts, "-")
}

func visibleTranscript(events []ChatEvent) string {
	var out strings.Builder
	var assistant strings.Builder

	flushAssistant := func() {
		text := strings.TrimSpace(assistant.String())
		if text == "" {
			assistant.Reset()
			return
		}
		out.WriteString("Assistant:\n")
		out.WriteString(text)
		out.WriteString("\n\n")
		assistant.Reset()
	}

	for _, ev := range events {
		switch ev.Type {
		case "user":
			flushAssistant()
			out.WriteString("User:\n")
			out.WriteString(strings.TrimSpace(ev.Text))
			out.WriteString("\n\n")
		case "assistant_text":
			assistant.WriteString(ev.Text)
		case "complete", "error":
			flushAssistant()
		}
	}
	flushAssistant()
	return out.String()
}
