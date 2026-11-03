package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/smtp"
	"github.com/futrx-com/remote.futrx.com/internal/integration/webpush"
	servicepermission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentaccountaccess "github.com/futrx-com/remote.futrx.com/internal/service/agent/accountaccess"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentcapability "github.com/futrx-com/remote.futrx.com/internal/service/agent/capability"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	servicepresence "github.com/futrx-com/remote.futrx.com/internal/service/presence"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
	servicetmux "github.com/futrx-com/remote.futrx.com/internal/service/tmux"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"

	emailoutbound "github.com/futrx-com/remote.futrx.com/internal/port/email/outbound"
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
	servicechat.TranscriptEventWindowSource
	servicechat.TranscriptProjectionSource
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
	ProjectShares     serviceshare.Repository
	Permissions       *servicepermission.Service
	Auth              *serviceauth.Service
	Users             serviceuser.Repository
	UserSettings      serviceusersettings.Repository
	Audit             serviceaudit.Store
	AuditRetention    int
	TwoFactor         serviceauth.TwoFactorStore
	SessionRegistry   serviceauth.SessionRegistryStore
	Push              PushStore
	Usage             serviceusage.Repository
	Email             emailoutbound.ConfigurationStore
	AgentQuota        agentquota.Repository
	AuthBaseURL       string
	ProjectContainers serviceproject.ContainerDependencies
	AgentContainers   provisioning.ContainerDependencies
	AgentModules      *agentmodule.Catalog
	AgentAPIKeys      agentauth.APIKeyStore
	AgentAccounts     agentauth.AccountStore
	AgentOptions      AgentOptions
	TmuxClient        TmuxClient
	ValidTmuxName     func(string) bool
	PromptStartGate   prompt.StartGate

	// Installable-application capabilities. When AppStore and
	// AppRegistry are set the Applications service is enabled.
	AppTurns     serviceapplications.AgentTurnRepository
	AppStore     serviceapplications.Store
	AppRegistry  serviceapplications.Registry
	AppInstaller serviceapplications.Installer
	AppPorts     serviceapplications.PortAllocator
	// AppBackends runs the application backends applications ship in their backend/ directory.
	// Leaving it nil keeps every other application capability working and
	// reports backend calls as unavailable.
	AppBackends serviceapplications.BackendHost
	// AppPackages is the writable half of the application catalog: the store
	// of packages an administrator uploaded. Nil leaves the catalog to whatever
	// the binary was built with.
	AppPackages serviceapplications.PackageCatalog
	// ApplicationLifecycle receives successful application catalog and
	// installed-copy transitions. Subscribers are wired at the process root.
	ApplicationLifecycle serviceapplications.ApplicationLifecyclePublisher
	// ApplicationEvents is the process-wide validated event stream routed to
	// subscribed application backends.
	ApplicationEvents serviceapplications.EventSource
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
	AccountAccess     *agentaccountaccess.Service
	Chats             *servicechat.Service
	ChatAccess        *servicechat.AccessService
	Projects          *serviceproject.Service
	Shares            *serviceshare.Service
	Prompt            *prompt.Service
	Agents            *agentmodule.Runtime
	AgentCapabilities *agentcapability.Service
	Runs              *runhub.Hub
	Workspace         *workspacehub.Hub
	Auth              *serviceauth.Service
	Users             *serviceuser.Service
	Permissions       *servicepermission.Service
	UserSettings      *serviceusersettings.Service
	Skills            *serviceskills.Catalog
	Access            *serviceauth.AccessVerifier
	Applications      *serviceapplications.Service
	Push              *servicepush.Service
	Presence          *servicepresence.Service
	Usage             *serviceusage.Service
	Email             *serviceemail.Service
	// Mailer is the entry point every other service uses to send email. It
	// hides credentials, MIME and HTML email markup behind a builder; see
	// service/email.Mail.
	Mailer     *serviceemail.Mailer
	AgentQuota *agentquota.Service
	Audit      *serviceaudit.Service
}

func New(ctx context.Context, deps Dependencies) (Services, error) {
	if err := deps.AgentContainers.Validate(); err != nil {
		return Services{}, fmt.Errorf("agent container dependencies: %w", err)
	}
	if deps.AgentModules == nil {
		return Services{}, errors.New("agent module catalog is required")
	}
	if deps.Auth == nil {
		return Services{}, errors.New("authentication service is required")
	}
	if err := deps.AgentModules.ValidateAccessGate(); err != nil {
		return Services{}, fmt.Errorf("agent module catalog: %w", err)
	}
	if deps.Permissions == nil {
		return Services{}, errors.New("permission service is required")
	}

	// The audit recorder is built first so every other service can take it.
	auditRetention := deps.AuditRetention
	if auditRetention == 0 {
		auditRetention = serviceaudit.DefaultRetentionMonths
	}
	auditLog := serviceaudit.New(deps.Audit, serviceaudit.WithRetentionMonths(auditRetention))
	auditLog.StartJanitor(ctx, 24*time.Hour)

	workspace := workspacehub.New()
	var runs *runhub.Hub
	// The notifier needs services that are built further down, so it is
	// created empty here and populated once they exist — the same late
	// binding the run hub uses above.
	presenceService := servicepresence.New()
	pushNotifier := &chatPushNotifier{chats: deps.Chats, projects: deps.Projects, presence: presenceService}
	chats := notifyingChatRepository{
		Repository: deps.Chats,
		workspace:  workspace,
		running: func(id servicechat.ID) bool {
			return runs != nil && runs.IsRunning(id)
		},
		push: pushNotifier,
	}
	// Authentication and authorization are built by the composition root and
	// injected, so this layer knows neither their stores nor their adapters.
	authService := deps.Auth
	serviceauth.WithAudit(auditLog)(authService)
	permissionService := deps.Permissions
	projects := notifyingProjectRepository{Repository: deps.Projects, workspace: workspace}
	projectService := serviceproject.New(
		projects,
		deps.ProjectContainers,
		deps.ProjectSecrets,
		deps.ProjectAccess,
		serviceproject.WithAuthorizer(permissionService),
		serviceproject.WithAudit(auditLog),
		serviceproject.WithChatCleanup(projectChatCleanup{
			chats: chats,
			cancel: func(ctx context.Context, id servicechat.ID) error {
				if runs == nil {
					return errors.New("run controller is unavailable")
				}
				return runs.Cancel(ctx, id)
			},
		}),
	)
	agentRuntime, err := deps.AgentModules.Build(agentmodule.BuildDependencies{
		Projects:              agentProjectResolver{projects: projectService},
		Containers:            deps.AgentContainers,
		APIKeys:               deps.AgentAPIKeys,
		Accounts:              agentauth.NewAccountVault(deps.AgentAccounts, permissionService).WithAudit(auditLog),
		AccountAuthorizer:     permissionService,
		CredentialSyncTimeout: deps.AgentOptions.CredentialSyncTimeout,
	})
	if err != nil {
		return Services{}, fmt.Errorf("build agent modules: %w", err)
	}
	accountAccess := agentaccountaccess.New(permissionService, agentRuntime.Bindings())
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
		servicechat.WithAudit(auditLog),
		servicechat.WithTranscriptEventSource(deps.Chats),
		servicechat.WithTranscriptEventWindowSource(deps.Chats),
		servicechat.WithTranscriptProjectionSource(deps.Chats),
		servicechat.WithCopiedEventAppender(chats),
		servicechat.WithSessionPolicy(agentRuntime),
		servicechat.WithProviderPolicy(agentRuntime),
		servicechat.WithAuthorizer(permissionService),
		servicechat.WithAccountAccess(accountAccess),
	)
	chatAccessService := servicechat.NewAccessService(chatService, projectService)
	pushService := newPush(deps.Push, deps.AuthBaseURL)
	userService := serviceuser.New(
		deps.Users,
		serviceuser.WithAudit(auditLog),
		serviceuser.WithRemovalCleanup(userRemovalCleanup{
			projects:        projectService,
			permissions:     permissionService,
			subscriptions:   deps.Push,
			twoFactor:       deps.TwoFactor,
			sessionRegistry: deps.SessionRegistry,
		}),
	)
	var usageService *serviceusage.Service
	promptOptions := []prompt.Option{
		prompt.WithAudit(auditLog),
		prompt.WithAgentPolicy(agentRuntime),
	}
	if deps.PromptStartGate != nil {
		promptOptions = append(promptOptions, prompt.WithStartGate(deps.PromptStartGate))
	}
	if deps.Usage != nil {
		usageService = serviceusage.New(deps.Usage, projectService, chats)
		promptOptions = append(promptOptions, prompt.WithUsageRecorder(usageService))
	}
	// The quota service is built even without a store: readings still show for
	// the life of the process, they just do not survive a restart.
	agentQuotaService := agentquota.New(ctx, deps.AgentQuota, agentRuntime.PlanUsageReaders()...)
	promptOptions = append(promptOptions, prompt.WithQuotaRecorder(agentQuotaService))
	promptOptions = append(promptOptions, prompt.WithAccountAccess(accountAccess))
	promptService := prompt.New(
		chats,
		deps.TmuxClient,
		projectService,
		runs,
		agentRuntime,
		promptOptions...,
	)
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
		accessVerifier = serviceauth.NewAccessVerifier(authService, projectService).WithWorkspaceAuthorizer(permissionService)
	}
	var shareService *serviceshare.Service
	if deps.ProjectShares != nil {
		shareService = serviceshare.New(deps.ProjectShares, projectService,
			serviceshare.WithProtectedPort(func(port int) bool {
				if deps.AppRegistry == nil {
					return false
				}
				for _, application := range deps.AppRegistry.List() {
					if application.Web != nil && application.Web.Port == port {
						return true
					}
				}
				return false
			}),
		)
	}
	var applicationsService *serviceapplications.Service
	if deps.AppStore != nil && deps.AppRegistry != nil {
		applicationsService = serviceapplications.New(
			deps.AppRegistry,
			deps.AppStore,
			deps.AppInstaller,
			projectContainersAdapter{projects: projectService},
			deps.AppPorts,
			serviceapplications.WithBackendHost(deps.AppBackends),
			serviceapplications.WithInstallRuns(ctx),
			serviceapplications.WithPackageCatalog(deps.AppPackages),
			serviceapplications.WithLifecyclePublisher(deps.ApplicationLifecycle),
			serviceapplications.WithEventSource(ctx, deps.ApplicationEvents),
			serviceapplications.WithAgentRuntime(ctx, serviceapplications.AgentDependencies{Chats: chatService, Projects: projectService, Identities: authService, Prompts: promptService, Turns: deps.AppTurns}),
			serviceapplications.WithAgentToolURL(deps.AuthBaseURL),
			serviceapplications.WithWorkspaceAuthorizer(permissionService),
			serviceapplications.WithAudit(auditLog),
		)
		projectService.SetContainerRestorer(applicationsService.RestoreProject)
	}

	if applicationsService != nil {
		if host, ok := deps.AppBackends.(serviceapplications.AgentRuntimeHost); ok {
			host.SetAgentTurns(applicationsService)
		}
		promptService.SetApplicationToolIssuer(applicationsService)
		applicationsService.StartBackground(ctx)
	}

	pushNotifier.push = pushService
	pushNotifier.audience.projects = projectService
	pushNotifier.audience.users = userService

	// The admin settings handler takes the Service (it manages the
	// configuration); every feature that merely wants to send mail takes the
	// Mailer facade. smtp.Client satisfies emailoutbound.Sender directly, so
	// composition needs no adapter between the two.
	emailService := serviceemail.New(deps.Email, smtp.New(constants.SMTPDialTimeout))
	mailer := serviceemail.NewMailer(emailService, emailDirectory{users: userService})

	return Services{
		AccountAccess:     accountAccess,
		Chats:             chatService,
		ChatAccess:        chatAccessService,
		Projects:          projectService,
		Shares:            shareService,
		Prompt:            promptService,
		Agents:            agentRuntime,
		AgentCapabilities: agentCapabilities,
		Runs:              runs,
		Workspace:         workspace,
		Auth:              authService,
		Users:             userService,
		Permissions:       permissionService,
		UserSettings:      userSettingsService,
		Skills:            skillCatalog,
		Access:            accessVerifier,
		Applications:      applicationsService,
		Push:              pushService,
		Presence:          presenceService,
		Usage:             usageService,
		Email:             emailService,
		Mailer:            mailer,
		AgentQuota:        agentQuotaService,
		Audit:             auditLog,
	}, nil
}

// projectContainersAdapter lets the applications service resolve and ready a
// project's container without importing the project service's concrete types.
type projectContainersAdapter struct {
	projects *serviceproject.Service
}

func (a projectContainersAdapter) ContainerName(ctx context.Context, projectID string) (string, error) {
	meta, err := a.projects.Get(ctx, serviceproject.ID(projectID))
	if err != nil {
		return "", err
	}
	return meta.Slug, nil
}

// EnsureRunning readies a container on behalf of an installed application
// that a caller has already been admitted to use, so it is trusted internal
// work rather than an explicit lifecycle action by that caller.
func (a projectContainersAdapter) EnsureRunning(ctx context.Context, projectID string) error {
	_, err := a.projects.Start(servicepermission.ContextWithSystemActor(ctx), serviceproject.ID(projectID))
	return err
}

func (a projectContainersAdapter) ListProjectIDs(ctx context.Context) ([]string, error) {
	projects, err := a.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, string(project.ID))
	}
	return ids, nil
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

func (s Services) AuthEnabled() bool {
	return s.Auth != nil
}

func (s Services) Reconcile(ctx context.Context) error {
	if s.Projects == nil {
		return nil
	}
	return s.Projects.Reconcile(ctx)
}

type chatProjectResolver struct {
	projects *serviceproject.Service
}

func (r chatProjectResolver) WorkspaceForProject(ctx context.Context, id servicechat.ProjectID) (string, error) {
	cwd, err := r.projects.WorkspaceForProject(ctx, serviceproject.ID(id))
	// The chat service has its own name for a project that is gone, so it can
	// tell that apart from a workspace it failed to read without importing the
	// project package.
	if errors.Is(err, serviceproject.ErrNotFound) {
		return "", servicechat.ErrProjectNotFound
	}
	return cwd, err
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
