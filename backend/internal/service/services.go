package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/googleoauth"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
	"github.com/futrx-com/remote.futrx.com/internal/service/schedulecapability"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
	servicetmux "github.com/futrx-com/remote.futrx.com/internal/service/tmux"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"
)

type AuthStore interface {
	serviceauth.Store
}

type TmuxClient interface {
	servicetmux.SessionClient
}

type Dependencies struct {
	Chats             servicechat.Repository
	Projects          serviceproject.Repository
	ProjectSecrets    serviceproject.SecretsRepository
	ProjectAccess     serviceproject.AccessRepository
	ProjectShares     serviceshare.Repository
	Schedules         serviceschedule.Repository
	Auth              AuthStore
	Users             serviceuser.Repository
	UserSettings      serviceusersettings.Repository
	AuthBaseURL       string
	ProjectContainers serviceproject.ContainerDependencies
	AgentContainers   provisioning.ContainerDependencies
	TmuxClient        TmuxClient
	ValidTmuxName     func(string) bool
	ScheduleLimits    ScheduleLimits
}

// ScheduleLimits mirrors the deployment's scheduled-task guardrails without
// coupling the service layer to the config package. Zero values disable a
// limit.
type ScheduleLimits struct {
	MinInterval        time.Duration
	MaxConcurrentRuns  int
	MaxTasksPerProject int
}

type Services struct {
	Chats        *servicechat.Service
	ChatAccess   *servicechat.AccessService
	Projects     *serviceproject.Service
	Shares       *serviceshare.Service
	Prompt       *prompt.Service
	Schedules    *serviceschedule.Service
	ScheduleCaps *schedulecapability.Registry
	AgentAuth    *agentauth.Registry
	Runs         *runhub.Hub
	Workspace    *workspacehub.Hub
	Auth         *serviceauth.Service
	Users        *serviceuser.Service
	UserSettings *serviceusersettings.Service
	Skills       *serviceskills.Catalog
	Tmux         *servicetmux.Service
	Access       *serviceauth.AccessVerifier
}

func New(ctx context.Context, deps Dependencies) (Services, error) {
	if err := deps.AgentContainers.Validate(); err != nil {
		return Services{}, fmt.Errorf("agent container dependencies: %w", err)
	}
	if deps.Schedules == nil {
		return Services{}, errors.New("scheduled task repository is required")
	}

	workspace := workspacehub.New()
	var runs *runhub.Hub
	chats := notifyingChatRepository{
		Repository: deps.Chats,
		workspace:  workspace,
		running: func(id servicechat.ID) bool {
			return runs != nil && runs.IsRunning(id)
		},
	}
	projects := notifyingProjectRepository{Repository: deps.Projects, workspace: workspace}
	definitions := agentDefinitions()
	profiles := profilesFromDefinitions(definitions)
	projectService := serviceproject.New(projects, deps.ProjectContainers, deps.ProjectSecrets, deps.ProjectAccess)
	projectService.StartAgentBrowserReaper(ctx, 20*time.Minute)
	runs = runhub.New(chats)
	runs.SetRunningSubscriber(func(id servicechat.ID, _ bool) {
		chats.publishChat(context.Background(), id)
	})

	var tmuxResolver servicechat.TmuxResolver
	if deps.TmuxClient != nil {
		tmuxResolver = chatTmuxResolver{client: deps.TmuxClient, validName: deps.ValidTmuxName}
	}

	chatService := servicechat.New(
		chats,
		chatProjectResolver{projects: projectService},
		tmuxResolver,
		runs,
	)
	chatAccessService := servicechat.NewAccessService(chatService, projectService)
	agents := agent.NewRegistry()
	agentAuth := agentauth.NewRegistry()
	for index, definition := range definitions {
		provider := definition.provider(projectService, deps.AgentContainers)
		if string(provider.ID()) != profiles[index].ID {
			return Services{}, fmt.Errorf(
				"agent registration mismatch: provider %q has profile %q",
				provider.ID(), profiles[index].ID,
			)
		}
		if err := agents.Register(provider); err != nil {
			return Services{}, err
		}
		authBinding := definition.authBinding()
		if authBinding.ID() != provider.ID() {
			return Services{}, fmt.Errorf(
				"agent auth registration mismatch: binding %q has provider %q",
				authBinding.ID(), provider.ID(),
			)
		}
		if err := agentAuth.Register(authBinding); err != nil {
			return Services{}, err
		}
	}
	userService := serviceuser.New(deps.Users)
	authService, err := newAuth(ctx, deps.Auth, userService, deps.AuthBaseURL)
	if err != nil {
		return Services{}, err
	}
	scheduleCaps := schedulecapability.New(deps.AuthBaseURL)
	promptService := prompt.New(
		chats,
		deps.TmuxClient,
		projectService,
		runs,
		agents,
		prompt.WithScheduleToolIssuer(scheduleCaps),
	)
	scheduleService := serviceschedule.New(
		deps.Schedules,
		chatService,
		projectService,
		authService,
		scheduledPromptExecutor{prompts: promptService},
		serviceschedule.WithMinInterval(deps.ScheduleLimits.MinInterval),
		serviceschedule.WithMaxConcurrentRuns(deps.ScheduleLimits.MaxConcurrentRuns),
		serviceschedule.WithMaxTasksPerProject(deps.ScheduleLimits.MaxTasksPerProject),
	)
	if err := scheduleService.Start(ctx); err != nil {
		return Services{}, fmt.Errorf("start scheduled tasks: %w", err)
	}
	userSettingsService := serviceusersettings.New(deps.UserSettings)
	skillService := serviceskills.New()
	skillCatalog := serviceskills.NewCatalog(skillService, projectService, authService)
	var accessVerifier *serviceauth.AccessVerifier
	if authService != nil {
		accessVerifier = serviceauth.NewAccessVerifier(authService, projectService)
	}
	var shareService *serviceshare.Service
	if deps.ProjectShares != nil {
		shareService = serviceshare.New(deps.ProjectShares, projectService)
	}
	var tmuxService *servicetmux.Service
	if deps.TmuxClient != nil {
		tmuxService = servicetmux.NewSessions(deps.TmuxClient)
	}

	return Services{
		Chats:        chatService,
		ChatAccess:   chatAccessService,
		Projects:     projectService,
		Shares:       shareService,
		Prompt:       promptService,
		Schedules:    scheduleService,
		ScheduleCaps: scheduleCaps,
		AgentAuth:    agentAuth,
		Runs:         runs,
		Workspace:    workspace,
		Auth:         authService,
		Users:        userService,
		UserSettings: userSettingsService,
		Skills:       skillCatalog,
		Tmux:         tmuxService,
		Access:       accessVerifier,
	}, nil
}

func (s Services) AuthEnabled() bool {
	return s.Auth != nil
}

func (s Services) Reconcile(ctx context.Context) error {
	if s.Projects == nil {
		return nil
	}
	return s.Projects.Reconcile(ctx)
}

type scheduledPromptExecutor struct {
	prompts *prompt.Service
}

func (e scheduledPromptExecutor) StartScheduledPrompt(
	ctx context.Context,
	task serviceschedule.Task,
	text string,
) (serviceschedule.RunHandle, error) {
	if e.prompts == nil {
		return nil, errors.New("prompt service is unavailable")
	}
	run, err := e.prompts.Start(prompt.StartInput{
		ChatID: task.ChatID,
		Prompt: text,
		Actor: prompt.Actor{
			Email: task.OwnerEmail,
		},
		ScheduledTaskID: string(task.ID),
		ScheduledRunID:  task.ActiveRunID,
		ParentContext:   ctx,
	}, nil)
	if errors.Is(err, prompt.ErrPromptAlreadyRunning) {
		return nil, serviceschedule.ErrExecutorBusy
	}
	if err != nil {
		return nil, err
	}

	done := make(chan serviceschedule.RunResult, 1)
	go func() {
		defer close(done)
		result, ok := <-run.Done
		if !ok {
			done <- serviceschedule.RunResult{
				Err: errors.New("prompt completion channel closed without a result"),
			}
			return
		}
		done <- serviceschedule.RunResult{
			Output: result.Output,
			Err:    result.Err,
		}
	}()
	return scheduledPromptHandle{done: done}, nil
}

type scheduledPromptHandle struct {
	done <-chan serviceschedule.RunResult
}

func (h scheduledPromptHandle) Done() <-chan serviceschedule.RunResult {
	return h.done
}

// userDirectoryAdapter wraps *serviceuser.Service to satisfy
// serviceauth.UserDirectory. AddBootstrapAdmin is the one method the auth
// service needs that the regular user.Service.Add doesn't quite cover (no
// "addedBy" since it's the bootstrap path).
type userDirectoryAdapter struct {
	users *serviceuser.Service
}

func (a userDirectoryAdapter) IsAdmin(ctx context.Context, email string) (bool, error) {
	return a.users.IsAdmin(ctx, email)
}

func (a userDirectoryAdapter) IsRegistered(ctx context.Context, email string) (bool, error) {
	return a.users.IsRegistered(ctx, email)
}

func (a userDirectoryAdapter) AddBootstrapAdmin(ctx context.Context, email string) error {
	_, err := a.users.Add(ctx, email, serviceuser.RoleAdmin, "")
	return err
}

func (a userDirectoryAdapter) FirstAdmin(ctx context.Context) (*serviceauth.UserDirectoryEntry, error) {
	list, err := a.users.List(ctx)
	if err != nil {
		return nil, err
	}
	var oldest *serviceuser.User
	for i := range list {
		u := &list[i]
		if u.Role != serviceuser.RoleAdmin {
			continue
		}
		if oldest == nil || u.AddedAt < oldest.AddedAt {
			oldest = u
		}
	}
	if oldest == nil {
		return nil, nil
	}
	return &serviceauth.UserDirectoryEntry{Email: oldest.Email}, nil
}

func newAuth(
	ctx context.Context,
	store AuthStore,
	users *serviceuser.Service,
	baseURL string,
) (*serviceauth.Service, error) {
	if store == nil {
		return nil, errors.New("authentication store is required")
	}
	baseURL, err := serviceauth.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	sessionKey, err := store.SessionKey(ctx)
	if err != nil {
		return nil, err
	}

	var directory serviceauth.UserDirectory
	if users != nil {
		directory = userDirectoryAdapter{users: users}
	}
	return serviceauth.New(
		ctx,
		store,
		directory,
		func(clientID, clientSecret, redirectURL string) serviceauth.OAuthProvider {
			return googleoauth.New(clientID, clientSecret, redirectURL)
		},
		baseURL,
		sessionKey,
	)
}

type chatProjectResolver struct {
	projects *serviceproject.Service
}

func (r chatProjectResolver) WorkspaceForProject(ctx context.Context, id servicechat.ProjectID) (string, error) {
	return r.projects.WorkspaceForProject(ctx, serviceproject.ID(id))
}

type chatTmuxResolver struct {
	client    TmuxClient
	validName func(string) bool
}

func (r chatTmuxResolver) ValidName(name string) bool {
	return r.validName != nil && r.validName(name)
}

func (r chatTmuxResolver) Cwd(ctx context.Context, session string) (string, error) {
	return r.client.Cwd(session)
}
