package prompt

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
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

// RunOutcome describes a settled agent run. Observers use it for out-of-band
// reporting such as outbound notifications; it never affects the run itself.
type RunOutcome struct {
	ChatID servicechat.ID
	RunID  uint64
	// Output is the concatenated assistant text produced by the run.
	Output    string
	Err       error
	Cancelled bool
	// ScheduledTaskID is set when the run was injected by the scheduler, so
	// observers can leave scheduled reporting to the schedule service.
	ScheduledTaskID string
}

// RunObserver receives out-of-band run signals. Implementations must not block:
// they are called on the run goroutine and on the provider event path.
type RunObserver interface {
	// RunSettled fires exactly once per run, after the provider returns.
	RunSettled(ctx context.Context, outcome RunOutcome)
	// RunToolStarted fires for every tool call the provider begins. Observers
	// decide which tool names mean the run is waiting on a human.
	RunToolStarted(ctx context.Context, chatID servicechat.ID, toolName string)
}

type Option func(*Service)

func WithScheduleToolIssuer(issuer ScheduleToolIssuer) Option {
	return func(service *Service) {
		service.scheduleTools = issuer
	}
}

// WithRunObserver installs an out-of-band observer of run lifecycle signals.
func WithRunObserver(observer RunObserver) Option {
	return func(service *Service) {
		service.observer = observer
	}
}

type Service struct {
	store         servicechat.Repository
	tmux          TmuxClient
	projects      ProjectResolver
	hub           *runhub.Hub
	agents        *agent.Registry
	scheduleTools ScheduleToolIssuer
	observer      RunObserver
}

func New(
	store servicechat.Repository,
	tmux TmuxClient,
	projects ProjectResolver,
	hub *runhub.Hub,
	agents *agent.Registry,
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
	go func() {
		defer close(done)
		defer rnr.hub.FinishRun(input.ChatID, runID)
		var output strings.Builder
		err := rnr.runPromptAs(
			ctx,
			input,
			func(ev ChatEvent) {
				rnr.hub.Emit(input.ChatID, ev)
				if ev.Type == "assistant_text" {
					output.WriteString(ev.Text)
				}
			},
			emitTransient,
		)
		rnr.observeSettled(parentCtx, RunOutcome{
			ChatID:          input.ChatID,
			RunID:           runID,
			Output:          output.String(),
			Err:             err,
			Cancelled:       errors.Is(ctx.Err(), context.Canceled),
			ScheduledTaskID: input.ScheduledTaskID,
		})
		done <- RunResult{Output: output.String(), Err: err}
	}()
	return RunHandle{ID: runID, Done: done}, nil
}

// observeSettled reports a settled run to the observer, if one is installed.
// The observer's own context governs its work, so a cancelled run still gets
// reported.
func (rnr *Service) observeSettled(ctx context.Context, outcome RunOutcome) {
	if rnr.observer == nil {
		return
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	rnr.observer.RunSettled(ctx, outcome)
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
	return rnr.runPromptAs(ctx, StartInput{ChatID: id, Prompt: prompt}, emit, emitTransient)
}

func (rnr *Service) runPromptAs(
	ctx context.Context,
	input StartInput,
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
	effectivePrompt := promptForMode(meta.Mode, prompt)
	enableBrowser := hasBrowserSkill(meta.SelectedSkills)
	if enableBrowser && meta.ProjectID != "" {
		stopBrowserKeepalive := rnr.keepAgentBrowserActivity(ctx, serviceproject.ID(meta.ProjectID))
		defer stopBrowserKeepalive()
	}
	if resumeID == "" {
		effectivePrompt = promptWithVisibleHistory(priorEvents, effectivePrompt)
	}
	effectivePrompt = promptWithSelectedSkills(providerID, promptSkills, effectivePrompt)

	provider := rnr.agents.Lookup(providerID)
	if provider == nil {
		err := errors.New(string(providerID) + " provider not configured")
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: err.Error()})
		return err
	}

	enableScheduleTools := hasScheduledTasksSkill(meta.SelectedSkills) || input.ScheduledTaskID != ""
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

	run := func(runPrompt, runResumeID string) error {
		return provider.Run(ctx, agent.RunRequest{
			Provider:       providerID,
			ConversationID: string(id),
			Prompt:         runPrompt,
			Cwd:            cwd,
			Model:          meta.Model,
			Mode:           meta.Mode,
			ResumeID:       runResumeID,
			ProjectID:      string(meta.ProjectID),
			Fork:           meta.ForkPending,
			Preferences: agent.RunPreferences{
				ReasoningEffort: agent.ReasoningEffort(meta.ReasoningEffort),
				ServiceTier:     agent.ServiceTier(meta.ServiceTier),
			},
			EnableBrowser:       enableBrowser,
			EnableScheduleTools: enableScheduleTools,
			RuntimeEnv:          runtimeEnv,
		}, func(ev agent.Event) {
			rnr.emitAgentEvent(ctx, id, ev, emit)
		})
	}

	err = run(effectivePrompt, resumeID)
	if errors.Is(err, agent.ErrSessionNotFound) && resumeID != "" {
		_, _ = rnr.store.Update(ctx, id, func(m *ChatMeta) {
			clearSessionIDForProvider(m, providerID)
			m.ForkPending = false
		})
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "system", Subtype: "session_recovered"})
		freshPrompt := promptForMode(meta.Mode, prompt)
		freshPrompt = promptWithVisibleHistory(priorEvents, freshPrompt)
		freshPrompt = promptWithSelectedSkills(providerID, promptSkills, freshPrompt)
		err = run(freshPrompt, "")
	}
	if err != nil && !errors.Is(err, agent.ErrRunFailed) {
		emit(ChatEvent{T: time.Now().UnixMilli(), Type: "error", Message: string(providerID) + " exit: " + err.Error()})
	}
	return err
}

func clearSessionIDForProvider(meta *ChatMeta, provider agent.ProviderID) {
	switch provider {
	case agent.ProviderCodex:
		meta.CodexSessionID = ""
	case agent.ProviderKimi:
		meta.KimiSessionID = ""
	case agent.ProviderAntigravity:
		meta.AntigravitySessionID = ""
	default:
		meta.ClaudeSessionID = ""
	}
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
	switch servicechat.NormalizeProvider(provider) {
	case servicechat.ProviderCodex:
		return agent.ProviderCodex
	case servicechat.ProviderKimi:
		return agent.ProviderKimi
	case servicechat.ProviderAntigravity:
		return agent.ProviderAntigravity
	default:
		return agent.ProviderClaude
	}
}

func sessionIDForProvider(meta ChatMeta, provider agent.ProviderID) string {
	switch provider {
	case agent.ProviderCodex:
		return meta.CodexSessionID
	case agent.ProviderKimi:
		return meta.KimiSessionID
	case agent.ProviderAntigravity:
		return meta.AntigravitySessionID
	default:
		return meta.ClaudeSessionID
	}
}

func promptForMode(mode, prompt string) string {
	switch mode {
	case "plan":
		return "Work in planning mode. Inspect enough context to be concrete, then propose the implementation plan before changing files. Avoid file edits until the user asks you to proceed.\n\n" + prompt
	case "review":
		return "Work in review mode. Prioritize bugs, behavioral regressions, missing tests, and risks. Put findings first with file and line references when available.\n\n" + prompt
	case "debug":
		return "Work in debugging mode. Reproduce or localize the issue first, explain the failing path, then make the smallest fix that addresses the root cause.\n\n" + prompt
	case "full-auto":
		return "Work in full-auto mode. Carry the task through implementation, verification, and a concise outcome unless you hit a hard blocker.\n\n" + prompt
	case "chat":
		return "Work in chat mode. Answer directly and avoid changing files unless the user clearly asks for implementation.\n\n" + prompt
	default:
		return prompt
	}
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

// hasBrowserSkill reports whether the user selected the `browser` skill for
// this prompt — the signal to wire the @playwright/mcp browser tools.
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

func promptWithSelectedSkills(provider agent.ProviderID, skills []servicechat.SkillRef, prompt string) string {
	if len(skills) == 0 {
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

		switch provider {
		case agent.ProviderClaude:
			triggers = append(triggers, "/"+name)
		case agent.ProviderCodex:
			triggers = append(triggers, "$"+name)
		case agent.ProviderKimi, agent.ProviderAntigravity:
			if name == scheduledTasksSkillName {
				triggers = append(
					triggers,
					"/workspace/.agents/skills/scheduled-tasks/SKILL.md",
				)
			}
		}
	}
	if len(triggers) == 0 {
		return prompt
	}

	switch provider {
	case agent.ProviderClaude:
		return strings.Join(triggers, "\n") + "\n\n" + prompt
	case agent.ProviderCodex:
		return "Use these Codex skills for this request: " + strings.Join(triggers, " ") + "\n\n" + prompt
	case agent.ProviderKimi, agent.ProviderAntigravity:
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
