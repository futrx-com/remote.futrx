package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/webpush"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentcapability "github.com/futrx-com/remote.futrx.com/internal/service/agent/capability"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicepresence "github.com/futrx-com/remote.futrx.com/internal/service/presence"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
	"github.com/futrx-com/remote.futrx.com/internal/service/schedulecapability"
	serviceserverinfo "github.com/futrx-com/remote.futrx.com/internal/service/serverinfo"
	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
	servicetmux "github.com/futrx-com/remote.futrx.com/internal/service/tmux"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
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

// ChatStore is the complete persistence capability required at composition;
// individual services receive only the narrower contracts they consume.
type ChatStore interface {
	servicechat.Repository
	servicechat.TranscriptEventSource
}

// PushStore persists Web Push registrations and the server's long-lived VAPID
// key pair. VAPIDKeys mints the pair on first use and returns the stored one
// thereafter; rotating it would invalidate every browser subscription.
type PushStore interface {
	servicepush.Repository
	removedUserSubscriptions
	VAPIDKeys(generate func() (private string, public string, err error)) (string, string, error)
}

type Dependencies struct {
	Chats             ChatStore
	Projects          serviceproject.Repository
	ProjectSecrets    serviceproject.SecretsRepository
	ProjectAccess     serviceproject.AccessRepository
	Schedules         serviceschedule.Repository
	Auth              AuthStore
	Users             serviceuser.Repository
	UserSettings      serviceusersettings.Repository
	TwoFactor         serviceauth.TwoFactorStore
	SessionRegistry   serviceauth.SessionRegistryStore
	Push              PushStore
	Usage             serviceusage.Repository
	ResourceSettings  serviceresources.Repository
	ResourceFleet     serviceresources.Fleet
	HostCollector     serviceserverinfo.Collector
	AuthBaseURL       string
	ProjectContainers serviceproject.ContainerDependencies
	AgentContainers   provisioning.ContainerDependencies
	AgentModules      *agentmodule.Catalog
	AgentAPIKeys      agentauth.APIKeyStore
	AgentOptions      AgentOptions
	AuthOptions       AuthOptions
	TmuxClient        TmuxClient
	ValidTmuxName     func(string) bool
	ScheduleLimits    ScheduleLimits
	PromptStartGate   prompt.StartGate
}

// ScheduleLimits mirrors the deployment's scheduled-task guardrails without
// coupling the service layer to the config package. Zero values disable a
// limit.
type ScheduleLimits struct {
	MinInterval        time.Duration
	MaxConcurrentRuns  int
	MaxTasksPerProject int
}

// AgentOptions mirrors application-wide agent policy without coupling the
// service layer to the config package.
type AgentOptions struct {
	CapabilityTimeout          time.Duration
	CapabilityCacheTTL         time.Duration
	DegradedCapabilityCacheTTL time.Duration
	CredentialSyncTimeout      time.Duration
	BrowserIdleTTL             time.Duration
}

// AuthOptions mirrors application-wide account security policy without
// coupling the service layer to the config package.
type AuthOptions struct {
	PendingLoginTTL     time.Duration
	EnrollmentTTL       time.Duration
	RecoveryCodeCount   int
	SessionHistoryLimit int
	SetupTokenTTL       time.Duration
}

type Services struct {
	Chats             *servicechat.Service
	ChatAccess        *servicechat.AccessService
	Projects          *serviceproject.Service
	Prompt            *prompt.Service
	Schedules         *serviceschedule.Service
	ScheduleCaps      *schedulecapability.Registry
	Agents            *agentmodule.Runtime
	AgentCapabilities *agentcapability.Service
	Runs              *runhub.Hub
	Workspace         *workspacehub.Hub
	Auth              *serviceauth.Service
	Users             *serviceuser.Service
	UserSettings      *serviceusersettings.Service
	Skills            *serviceskills.Catalog
	Tmux              *servicetmux.Service
	Access            *serviceauth.AccessVerifier
	Push              *servicepush.Service
	Presence          *servicepresence.Service
	Usage             *serviceusage.Service
	Resources         *serviceresources.Service
}

func New(ctx context.Context, deps Dependencies) (Services, error) {
	if err := deps.AgentContainers.Validate(); err != nil {
		return Services{}, fmt.Errorf("agent container dependencies: %w", err)
	}
	if deps.AgentModules == nil {
		return Services{}, errors.New("agent module catalog is required")
	}
	if deps.Auth != nil {
		if err := deps.AgentModules.ValidateAccessGate(); err != nil {
			return Services{}, fmt.Errorf("agent module catalog: %w", err)
		}
	}
	if deps.Schedules == nil {
		return Services{}, errors.New("scheduled task repository is required")
	}

	workspace := workspacehub.New()
	var runs *runhub.Hub
	// The notifier needs services that are built further down, so it is
	// created empty here and populated once they exist — the same late
	// binding the run hub uses above.
	presenceService := servicepresence.New()
	pushNotifier := &chatPushNotifier{chats: deps.Chats, presence: presenceService}
	chats := notifyingChatRepository{
		Repository: deps.Chats,
		workspace:  workspace,
		running: func(id servicechat.ID) bool {
			return runs != nil && runs.IsRunning(id)
		},
		push: pushNotifier,
	}
	projects := notifyingProjectRepository{Repository: deps.Projects, workspace: workspace}

	// The fleet resource policy is loaded (or derived from host capacity on
	// first run) before any project can launch, so the very first container
	// of a fresh install already lands inside a host-aware envelope.
	resourceService := serviceresources.New(
		deps.ResourceSettings,
		hostFactsAdapter{collector: deps.HostCollector},
		deps.ResourceFleet,
	)
	if deps.ResourceSettings != nil {
		if err := resourceService.Ensure(ctx); err != nil {
			log.Printf("resources: converge fleet defaults: %v", err)
		}
		policy := resourcePolicyAdapter{resources: resourceService}
		deps.ProjectContainers.Policy = policy
		deps.ProjectContainers.Admission = policy
	}
	projectService := serviceproject.New(projects, deps.ProjectContainers, deps.ProjectSecrets, deps.ProjectAccess)
	agentRuntime, err := deps.AgentModules.Build(agentmodule.BuildDependencies{
		Projects:              agentProjectResolver{projects: projectService},
		Containers:            deps.AgentContainers,
		APIKeys:               deps.AgentAPIKeys,
		CredentialSyncTimeout: deps.AgentOptions.CredentialSyncTimeout,
	})
	if err != nil {
		return Services{}, fmt.Errorf("build agent modules: %w", err)
	}
	projectService.StartAgentBrowserReaper(ctx, deps.AgentOptions.BrowserIdleTTL)
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
		servicechat.WithTranscriptEventSource(deps.Chats),
		servicechat.WithCopiedEventAppender(chats),
		servicechat.WithSessionPolicy(agentRuntime),
		servicechat.WithProviderPolicy(agentRuntime),
	)
	chatAccessService := servicechat.NewAccessService(chatService, projectService)
	pushService := newPush(deps.Push, deps.AuthBaseURL)
	userService := serviceuser.New(
		deps.Users,
		serviceuser.WithRemovalCleanup(userRemovalCleanup{
			projects:        projectService,
			subscriptions:   deps.Push,
			twoFactor:       deps.TwoFactor,
			sessionRegistry: deps.SessionRegistry,
		}),
	)
	authService, err := newAuth(
		ctx,
		deps.Auth,
		userService,
		deps.AuthBaseURL,
		deps.TwoFactor,
		deps.SessionRegistry,
		deps.AuthOptions,
	)
	if err != nil {
		return Services{}, err
	}
	scheduleCaps := schedulecapability.New(deps.AuthBaseURL)
	var usageService *serviceusage.Service
	promptOptions := []prompt.Option{
		prompt.WithScheduleToolIssuer(scheduleCaps),
		prompt.WithAgentPolicy(agentRuntime),
	}
	if deps.PromptStartGate != nil {
		promptOptions = append(promptOptions, prompt.WithStartGate(deps.PromptStartGate))
	}
	if deps.Usage != nil {
		usageService = serviceusage.New(deps.Usage, projectService, chats)
		promptOptions = append(promptOptions, prompt.WithUsageRecorder(usageService))
	}
	promptService := prompt.New(
		chats,
		deps.TmuxClient,
		projectService,
		runs,
		agentRuntime,
		promptOptions...,
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
	userSettingsService := serviceusersettings.New(
		deps.UserSettings,
		serviceusersettings.WithProviderCatalog(agentRuntime),
	)
	skillService := serviceskills.New(serviceskills.WithProviderCatalog(agentRuntime))
	skillCatalog := serviceskills.NewCatalog(skillService, projectService, authService)
	agentCapabilities := agentcapability.New(
		agentRuntime,
		projectService,
		authService,
		agentcapability.Settings{
			CapabilityTimeout:          deps.AgentOptions.CapabilityTimeout,
			CapabilityCacheTTL:         deps.AgentOptions.CapabilityCacheTTL,
			DegradedCapabilityCacheTTL: deps.AgentOptions.DegradedCapabilityCacheTTL,
		},
		agentcapability.WithModulePolicy(agentRuntime),
	)
	var accessVerifier *serviceauth.AccessVerifier
	if authService != nil {
		accessVerifier = serviceauth.NewAccessVerifier(authService, projectService)
	}
	var tmuxService *servicetmux.Service
	if deps.TmuxClient != nil {
		tmuxService = servicetmux.NewSessions(deps.TmuxClient)
	}

	pushNotifier.push = pushService
	pushNotifier.audience.projects = projectService
	pushNotifier.audience.users = userService

	return Services{
		Chats:             chatService,
		ChatAccess:        chatAccessService,
		Projects:          projectService,
		Prompt:            promptService,
		Schedules:         scheduleService,
		ScheduleCaps:      scheduleCaps,
		Agents:            agentRuntime,
		AgentCapabilities: agentCapabilities,
		Runs:              runs,
		Workspace:         workspace,
		Auth:              authService,
		Users:             userService,
		UserSettings:      userSettingsService,
		Skills:            skillCatalog,
		Tmux:              tmuxService,
		Access:            accessVerifier,
		Push:              pushService,
		Presence:          presenceService,
		Usage:             usageService,
		Resources:         resourceService,
	}, nil
}

// newPush builds the Web Push service. A deployment without a usable VAPID key
// simply has notifications switched off; it is not a reason to refuse to boot.
func newPush(store PushStore, baseURL string) *servicepush.Service {
	if store == nil {
		return servicepush.New(nil, nil)
	}
	private, public, err := store.VAPIDKeys(func() (string, string, error) {
		key, err := webpush.GenerateVAPIDKey()
		if err != nil {
			return "", "", err
		}
		return key.PrivateKeyBase64(), key.PublicKeyBase64(), nil
	})
	if err != nil {
		log.Printf("push: notifications disabled: %v", err)
		return servicepush.New(store, nil)
	}
	key, err := webpush.ParseVAPIDKey(private, public)
	if err != nil {
		log.Printf("push: notifications disabled: %v", err)
		return servicepush.New(store, nil)
	}
	client, err := webpush.NewClient(key, baseURL)
	if err != nil {
		log.Printf("push: notifications disabled: %v", err)
		return servicepush.New(store, nil)
	}
	return servicepush.New(store, webPushSender{client: client})
}

// hostFactsAdapter narrows the server-info collector to the capacity facts the
// resource policy needs, so the numbers an admin reads on the Info page and
// the numbers the aggregate guard enforces come from one source.
type hostFactsAdapter struct {
	collector serviceserverinfo.Collector
}

func (a hostFactsAdapter) Facts(ctx context.Context) serviceresources.HostFacts {
	if a.collector == nil {
		return serviceresources.HostFacts{}
	}
	snapshot := a.collector.Collect(ctx, time.Now())
	return serviceresources.HostFacts{
		MemoryBytes: snapshot.Memory.TotalBytes,
		CPUs:        snapshot.CPU.LogicalCores,
		DiskBytes:   snapshot.Storage.TotalBytes,
	}
}

// resourcePolicyAdapter translates between the resource service's own policy
// vocabulary and the ports the project service declares, keeping neither
// service dependent on the other's types.
type resourcePolicyAdapter struct {
	resources *serviceresources.Service
}

func (a resourcePolicyAdapter) Policy(ctx context.Context) serviceproject.ContainerPolicySnapshot {
	view := a.resources.Get(ctx)
	return serviceproject.ContainerPolicySnapshot{
		Defaults:             projectLimits(view.Settings.Defaults),
		MaxOverride:          projectLimits(view.Settings.MaxProjectOverride),
		MaxRunningContainers: view.Settings.MaxRunningContainers,
		Host: serviceproject.HostCapacity{
			MemoryBytes:        view.Host.MemoryBytes,
			CPUs:               view.Host.CPUs,
			DiskBytes:          view.Host.DiskBytes,
			ReserveMemoryBytes: view.Host.ReserveMemoryBytes,
			BudgetMemoryBytes:  view.Host.BudgetMemoryBytes,
			CommittedBytes:     view.Host.CommittedBytes,
			RunningContainers:  view.Host.RunningContainers,
		},
		DiskQuota: serviceproject.DiskQuotaSupport{
			Supported: view.DiskQuota.Supported,
			Pool:      view.DiskQuota.Pool,
			Driver:    view.DiskQuota.Driver,
			Detail:    view.DiskQuota.Detail,
		},
	}
}

func (a resourcePolicyAdapter) Validate(ctx context.Context, limits serviceproject.ContainerLimits) error {
	cores := 0.0
	if limits.CPU != "" {
		parsed, err := strconv.ParseFloat(limits.CPU, 64)
		if err != nil {
			return serviceproject.ErrInvalidLimits
		}
		cores = parsed
	}
	return a.resources.ValidateOverride(ctx, limits.Memory, cores, limits.Disk)
}

func (a resourcePolicyAdapter) AuthorizeStart(ctx context.Context, containerName, memoryLimit string, force bool) error {
	return a.resources.AuthorizeStart(ctx, containerName, memoryLimit, force)
}

func projectLimits(limits serviceresources.Limits) serviceproject.ContainerLimits {
	return serviceproject.ContainerLimits{
		CPU:    serviceproject.FormatCores(limits.CPU),
		Memory: limits.Memory,
		Disk:   limits.Disk,
	}
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
	if errors.Is(err, prompt.ErrPromptAlreadyRunning) || errors.Is(err, prompt.ErrMaintenance) {
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
