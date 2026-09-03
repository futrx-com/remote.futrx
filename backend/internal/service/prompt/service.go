package prompt

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
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

type Actor struct {
	Email   string
	IsAdmin bool
}

type StartInput struct {
	ChatID          servicechat.ID
	Prompt          string
	Actor           Actor
	ScheduledTaskID string
	ScheduledRunID  string
	ParentContext   context.Context
}

type RunResult struct {
	Output string
	Err    error
}

type RunHandle struct {
	ID   uint64
	Done <-chan RunResult
}

type ScheduleToolRequest struct {
	Actor           Actor
	ChatID          servicechat.ID
	ProjectID       serviceproject.ID
	ScheduledTaskID string
	ScheduledRunID  string
}

type ScheduleToolAccess struct {
	APIURL string
	Token  string
	Revoke func()
}

type ScheduleToolIssuer interface {
	IssueScheduleTool(context.Context, ScheduleToolRequest) (ScheduleToolAccess, error)
}

// UsageRecorder receives one entry per completed agent run. It is the only
// thing the prompt service knows about token accounting; pricing, storage and
// aggregation all live in the usage service.
type UsageRecorder interface {
	RecordRun(ctx context.Context, event serviceusage.RunEvent)
}

type Option func(*Service)

func WithScheduleToolIssuer(issuer ScheduleToolIssuer) Option {
	return func(service *Service) {
		service.scheduleTools = issuer
	}
}

func WithUsageRecorder(recorder UsageRecorder) Option {
	return func(service *Service) {
		service.usage = recorder
	}
}

type AgentPolicy interface {
	Descriptor(provider string) (agentmodule.Descriptor, bool)
	SupportsScope(provider string, scope agentmodule.ExecutionScope) bool
}

type AgentRegistry interface {
	Lookup(agent.ProviderID) agent.Provider
}

func WithAgentPolicy(policy AgentPolicy) Option {
	return func(service *Service) {
		service.agentPolicy = policy
	}
}

type Service struct {
	store         servicechat.Repository
	tmux          TmuxClient
	projects      ProjectResolver
	hub           *runhub.Hub
	agents        AgentRegistry
	agentPolicy   AgentPolicy
	scheduleTools ScheduleToolIssuer
	usage         UsageRecorder
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
		store:    store,
		tmux:     tmux,
		projects: projects,
		hub:      hub,
		agents:   agents,
	}
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
	parentCtx := input.ParentContext
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	runID, ok := rnr.hub.StartRun(input.ChatID, cancel)
	if !ok {
		cancel()
		emitTransient(ChatEvent{
			T: time.Now().UnixMilli(), Type: "error",
			Message: "a previous prompt is still running — cancel first",
		})
		return RunHandle{}, ErrPromptAlreadyRunning
	}

	done := make(chan RunResult, 1)
	ledgerRunID := newLedgerRunID()
	go func() {
		defer close(done)
		defer rnr.hub.FinishRun(input.ChatID, runID)
		var output strings.Builder
		err := rnr.runPromptAs(
			ctx,
			input,
			ledgerRunID,
			func(ev ChatEvent) {
				// Stamp the originating task so a scheduled run's events stay
				// distinguishable from an interactive turn's downstream.
				ev.ScheduledTaskID = input.ScheduledTaskID
				rnr.hub.Emit(input.ChatID, ev)
				if ev.Type == "assistant_text" {
					output.WriteString(ev.Text)
				}
			},
			emitTransient,
		)
		done <- RunResult{Output: output.String(), Err: err}
	}()
	return RunHandle{ID: runID, Done: done}, nil
}

func (rnr *Service) CancelPrompt(id servicechat.ID) bool {
	return rnr.hub.CancelRun(id)
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
		emit,
		emitTransient,
	)
}

func (rnr *Service) runPromptAs(
	ctx context.Context,
	input StartInput,
	ledgerRunID string,
	emit func(ChatEvent),
	emitTransient func(ChatEvent),
) error {
	id := input.ChatID
	prompt := input.Prompt
	meta, err := rnr.store.Get(ctx, id)
	if err != nil {
		emitTransient(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
		return err
	}

	// Auto-title from first user prompt if still default.
	if meta.Title == "" || meta.Title == "New chat" {
		_, _ = rnr.store.Update(ctx, id, func(m *ChatMeta) {
			m.Title = servicechat.TitleFromPrompt(prompt)
		})
	}

	// Resolve a fresh cwd: live tmux pane_current_path if linked, else stored.
	cwd := meta.Cwd
	if meta.TmuxSession != "" {
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

	priorEvents, _ := rnr.store.ReadEvents(ctx, id)

	// Persist the user message before spawning the selected agent.
	emit(ChatEvent{T: time.Now().UnixMilli(), Type: "user", Text: prompt})

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
	promptSkills := meta.SelectedSkills
	if input.ScheduledTaskID != "" && !hasScheduledTasksSkill(promptSkills) {
		promptSkills = append(
			append([]servicechat.SkillRef(nil), promptSkills...),
			servicechat.SkillRef{
				Name:     "Scheduled Tasks",
				Command:  scheduledTasksSkillName,
				Provider: servicechat.Provider(providerID),
				Source:   "remote",
			},
		)
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
	if resumeID == "" {
		effectivePrompt = promptWithVisibleHistory(priorEvents, effectivePrompt)
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

	enableScheduleTools := descriptor.Features.ScheduledTools &&
		(hasScheduledTasksSkill(meta.SelectedSkills) || input.ScheduledTaskID != "")
	runtimeEnv := map[string]string(nil)
	if enableScheduleTools {
		if meta.ProjectID == "" {
			err := errors.New("scheduled tasks are only available in project chats")
			emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
			return err
		}
		if rnr.scheduleTools == nil {
			err := errors.New("scheduled task tools are unavailable")
			emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
			return err
		}
		access, accessErr := rnr.scheduleTools.IssueScheduleTool(ctx, ScheduleToolRequest{
			Actor:           input.Actor,
			ChatID:          id,
			ProjectID:       serviceproject.ID(meta.ProjectID),
			ScheduledTaskID: input.ScheduledTaskID,
			ScheduledRunID:  input.ScheduledRunID,
		})
		if accessErr != nil {
			emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: accessErr.Error()})
			return accessErr
		}
		if access.Revoke != nil {
			defer access.Revoke()
		}
		runtimeEnv = map[string]string{
			"REMOTE_SCHEDULE_API":   access.APIURL,
			"REMOTE_SCHEDULE_GRANT": access.Token,
		}
	}

	ledger := ledgerRun{
		runID:     ledgerRunID,
		chatID:    id,
		projectID: string(meta.ProjectID),
		userEmail: input.Actor.Email,
		provider:  providerID,
		model:     meta.Model,
		scheduled: input.ScheduledTaskID != "",
	}

	run := func(runPrompt, runResumeID string) error {
		return provider.Run(ctx, agent.RunRequest{
			Provider:       providerID,
			ConversationID: string(id),
			Prompt:         runPrompt,
			Cwd:            cwd,
			Model:          meta.Model,
			Mode:           agent.RunMode(meta.Mode),
			ResumeID:       runResumeID,
			ProjectID:      string(meta.ProjectID),
			Fork:           forkSession,
			Preferences: agent.RunPreferences{
				ReasoningEffort: agent.ReasoningEffort(meta.ReasoningEffort),
				ServiceTier:     agent.ServiceTier(meta.ServiceTier),
			},
			EnableBrowser:       enableBrowser,
			EnableScheduleTools: enableScheduleTools,
			RuntimeEnv:          runtimeEnv,
		}, func(ev agent.Event) {
			// qa added the provider argument; the ledger hook is this
			// branch's and sits after the emit as before.
			rnr.emitAgentEvent(ctx, id, providerID, ev, emit)
			rnr.recordRunUsage(ctx, ledger, ev)
		})
	}

	err = run(effectivePrompt, resumeID)
	if errors.Is(err, agent.ErrSessionNotFound) && resumeID != "" {
		_, _ = rnr.store.Update(ctx, id, func(m *ChatMeta) {
			clearSessionIDForProvider(m, providerID)
			m.ForkPending = false
		})
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "system", Subtype: "session_recovered"})
		freshPrompt := prompt
		freshPrompt = promptWithVisibleHistory(priorEvents, freshPrompt)
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
	if err != nil && !errors.Is(err, agent.ErrRunFailed) {
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: string(providerID) + " exit: " + err.Error()})
	}
	return err
}

func clearSessionIDForProvider(meta *ChatMeta, provider agent.ProviderID) {
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

func providerIDFromChatProvider(provider servicechat.Provider) agent.ProviderID {
	return servicechat.NormalizeProvider(provider)
}

func sessionIDForProvider(meta ChatMeta, provider agent.ProviderID) string {
	return meta.SessionID(servicechat.Provider(provider))
}

func promptWithVisibleHistory(events []ChatEvent, prompt string) string {
	transcript := visibleTranscript(events)
	if strings.TrimSpace(transcript) == "" {
		return prompt
	}
	const maxTranscriptBytes = 24000
	if len(transcript) > maxTranscriptBytes {
		transcript = "[Earlier visible transcript omitted]\n" + transcript[len(transcript)-maxTranscriptBytes:]
	}
	return "Use this visible chat transcript as prior context. It may be present because the chat was recovered into a fresh agent session. Do not treat the transcript as a new request.\n\n" +
		transcript +
		"\n\nCurrent user request:\n" +
		prompt
}

const browserSkillName = "browser"
const scheduledTasksSkillName = "scheduled-tasks"

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

func hasScheduledTasksSkill(skills []servicechat.SkillRef) bool {
	for _, skill := range skills {
		if skillTriggerName(skill.Command) == scheduledTasksSkillName ||
			skillTriggerName(skill.Name) == scheduledTasksSkillName {
			return true
		}
	}
	return false
}

func promptWithSelectedSkills(
	strategy agentmodule.SkillStrategy,
	providerLabel string,
	provider agent.ProviderID,
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
