// remote.futrx is the self-hosted control plane for configured coding agents
// and their isolated project workspaces.
//
// Backend serves:
//   - Static SPA (Preact/Vite bundle) embedded via go:embed
//   - HTTP APIs for users, agents, chats, projects, files, and operations
//   - WebSockets for workspace state, agent runs, auth status, and terminals

package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	remote "github.com/futrx-com/remote.futrx.com"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/config"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	applicationbackends "github.com/futrx-com/remote.futrx.com/internal/integration/applications"
	containerapplications "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	"github.com/futrx-com/remote.futrx.com/internal/integration/gitcli"
	"github.com/futrx-com/remote.futrx.com/internal/integration/hostfs"
	"github.com/futrx-com/remote.futrx.com/internal/integration/hostinfo"
	"github.com/futrx-com/remote.futrx.com/internal/integration/lxc"
	"github.com/futrx-com/remote.futrx.com/internal/integration/tmuxcli"
	"github.com/futrx-com/remote.futrx.com/internal/integration/updatecli"
	"github.com/futrx-com/remote.futrx.com/internal/lifecycle"
	service "github.com/futrx-com/remote.futrx.com/internal/service"
	servicegithistory "github.com/futrx-com/remote.futrx.com/internal/service/githistory"
	servicemaintenance "github.com/futrx-com/remote.futrx.com/internal/service/maintenance"
	serviceselfupdate "github.com/futrx-com/remote.futrx.com/internal/service/selfupdate"
	serviceserverinfo "github.com/futrx-com/remote.futrx.com/internal/service/serverinfo"
	serviceworkspacefiles "github.com/futrx-com/remote.futrx.com/internal/service/workspacefiles"
	serviceworkspaceide "github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
	"github.com/futrx-com/remote.futrx.com/internal/stores"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/transport"
	"github.com/futrx-com/remote.futrx.com/internal/version"
)

func main() {
	// This executable is the process composition root. The sections below
	// follow dependency direction from configuration and outbound adapters to
	// application policy, inbound transport, and process-owned runtime work.

	////////////////////////////////////////
	// Configuration
	////////////////////////////////////////
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.Load()

	if runCLICommand(ctx, cfg, os.Args) {
		cancel()
		return
	}
	publicHostname, err := config.PublicHostname(cfg.BaseURL)
	if err != nil {
		log.Fatalf("configure public hostname: %v", err)
	}

	////////////////////////////////////////
	// Container and workspace capabilities
	////////////////////////////////////////
	agentModules, err := config.NewAgentModules()
	if err != nil {
		log.Fatalf("configure agent modules: %v", err)
	}
	// Uploaded application packages live in the server's state directory, not
	// in the binary and not in the checkout. That is what makes them survive an
	// update: updating replaces the program and its built-in catalog, and never
	// touches this directory or the instances installed from it.
	appPackages, err := containerapplications.NewPackageStore(
		filepath.Join(cfg.DataDir, "app-packages"),
	)
	if err != nil {
		log.Fatalf("open uploaded application packages: %v", err)
	}
	appRegistry, err := containerapplications.NewRegistry(
		containerapplications.EmbeddedCatalog(),
		appPackages,
	)
	if err != nil {
		log.Fatalf("load application catalog: %v", err)
	}
	// This dynamic bus is the one process-wide boundary shared by core lifecycle
	// publishers and manifest-declared application publishers/subscribers.
	applicationEvents := lifecycle.NewEventBus(ctx)
	// Application backends are compiled from the catalog's embedded Go source and
	// run as child processes. Their binaries and per-instance data live beside
	// the rest of the server's state so an uninstall leaves nothing behind.
	appBackends := applicationbackends.New(
		filepath.Join(cfg.DataDir, "applications"),
		appRegistry,
		applicationbackends.Options{
			GoTool: cfg.Applications.GoTool,
			Events: applicationEvents,
		},
	)
	// Service-owned workers are closed after construction below. These defers
	// then cancel the shared lifecycle, wait for dynamic dispatch, and finally
	// stop child backends. Defers run in last-in, first-out order.
	defer appBackends.Shutdown()
	defer applicationEvents.Close()
	defer cancel()

	containerStack := config.NewContainerStack(
		lxc.New(),
		agentModules.Profiles(),
		config.ContainerStackOptions{
			AgentInstructions: provisioning.InstructionsTemplate(publicHostname),
			AppRegistry:       appRegistry,
			DataDir:           cfg.DataDir,
		},
	)

	////////////////////////////////////////
	// Persistence
	////////////////////////////////////////
	storeSet, err := stores.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("init stores: %v", err)
	}

	////////////////////////////////////////
	// Application services
	////////////////////////////////////////
	maintenanceGuard := servicemaintenance.New(cfg.DataDir)

	// Lifecycle publishers are process-wide. Producers receive only the
	// publishing capability declared by their own service contract.
	updateLifecycle := lifecycle.NewUpdatePublisher()
	applicationLifecycle := lifecycle.NewApplicationPublisher()
	applicationEventBridge := lifecycle.NewApplicationEventBridge(applicationEvents)
	unsubscribeApplicationCatalog := applicationLifecycle.SubscribeCatalog(applicationEventBridge)
	defer unsubscribeApplicationCatalog()
	unsubscribeApplicationInstances := applicationLifecycle.SubscribeInstances(applicationEventBridge)
	defer unsubscribeApplicationInstances()
	selfUpdateService := serviceselfupdate.New(
		version.Version,
		cfg.InstallDir,
		cfg.DataDir,
		updatecli.New(),
		updateLifecycle,
	)

	tmuxClient := tmuxcli.New()
	serviceSet, err := service.New(ctx, service.Dependencies{
		Chats:             storeSet.Chats,
		Projects:          storeSet.Projects,
		ProjectSecrets:    storeSet.ProjectSecrets,
		ProjectAccess:     storeSet.ProjectAccess,
		ProjectShares:     storeSet.ProjectShares,
		Schedules:         storeSet.Schedules,
		Auth:              storeSet.Auth,
		Users:             storeSet.Users,
		UserSettings:      storeSet.UserSettings,
		TwoFactor:         storeSet.TwoFactor,
		SessionRegistry:   storeSet.SessionRegistry,
		Push:              storeSet.Push,
		Usage:             storeSet.Usage,
		AuthBaseURL:       cfg.BaseURL,
		ProjectContainers: containerStack.ProjectDependencies(),
		AgentContainers:   containerStack.AgentDependencies(),
		AgentModules:      agentModules,
		AgentAPIKeys:      storeSet.AgentAPIKeys,
		AgentAccounts:     storeSet.AgentAccounts,
		AgentOptions: service.AgentOptions{
			CapabilityTimeout:          cfg.Agent.CapabilityTimeout,
			CapabilityCacheTTL:         cfg.Agent.CapabilityCacheTTL,
			DegradedCapabilityCacheTTL: cfg.Agent.DegradedCapabilityCacheTTL,
			CredentialSyncTimeout:      cfg.Agent.CredentialSyncTimeout,
			BrowserIdleTTL:             cfg.Agent.BrowserIdleTTL,
		},
		AuthOptions: service.AuthOptions{
			PendingLoginTTL:     cfg.Auth.PendingLoginTTL,
			EnrollmentTTL:       cfg.Auth.EnrollmentTTL,
			RecoveryCodeCount:   cfg.Auth.RecoveryCodeCount,
			SessionHistoryLimit: cfg.Auth.SessionHistoryLimit,
			SetupTokenTTL:       cfg.Auth.SetupTokenTTL,
		},
		TmuxClient:    tmuxClient,
		ValidTmuxName: tmuxcli.ValidName,
		ScheduleLimits: service.ScheduleLimits{
			MinInterval:        cfg.Schedule.MinInterval,
			MaxConcurrentRuns:  cfg.Schedule.MaxConcurrentRuns,
			MaxTasksPerProject: cfg.Schedule.MaxTasksPerProject,
		},
		AppStore:             storeSet.Applications,
		AppDefaultStore:      storeSet.Applications,
		AppRegistry:          appRegistry,
		AppInstaller:         containerStack.AppInstaller,
		AppPorts:             containerStack.AppPorts,
		AppBackends:          appBackends,
		AppPackages:          appRegistry,
		ApplicationLifecycle: applicationLifecycle,
		ApplicationEvents:    applicationEvents,
		PromptStartGate:      maintenanceGuard,
	})
	if err != nil {
		log.Fatalf("init services: %v", err)
	}
	// This defer is registered after the bus/host defers above, so routing fully
	// unsubscribes and exits before the bus closes and backend children stop.
	if serviceSet.Applications != nil {
		defer serviceSet.Applications.Close()
	}
	// Terminal self-update events are reconciled from disk so a backend
	// replacement can deliver the completion started by its predecessor.
	if err := selfUpdateService.StartLifecycleReconciler(ctx); err != nil {
		log.Printf("self-update: lifecycle reconcile warning: %v", err)
	}
	log.Printf(
		"auth: local admin enabled; Google OAuth configured=%t; BASE_URL=%s",
		serviceSet.Auth.GoogleOAuthEnabled(),
		cfg.BaseURL,
	)
	// On a first boot nobody exists to authorise the local-admin claim, so the
	// setup token is minted and printed here and nowhere else: the operator's
	// terminal is the one channel a passer-by loading the page cannot reach.
	// Issuing on every gated start also rotates it, so a token that leaked
	// before a restart is already dead.
	announceSetupToken(ctx, serviceSet.Auth, cfg.BaseURL, log.Writer())
	if err := serviceSet.Reconcile(ctx); err != nil {
		log.Printf("services: reconcile warning: %v", err)
	}

	////////////////////////////////////////
	// HTTP transport
	////////////////////////////////////////
	static, err := fs.Sub(remote.PublicFS, "public")
	if err != nil {
		log.Fatal(err)
	}
	codeServerBaseURL, err := config.CodeServerBaseURL(cfg.BaseURL)
	if err != nil {
		log.Fatalf("configure IDE URL: %v", err)
	}

	handler, err := transport.NewHTTPHandler(transport.Dependencies{
		Services:       serviceSet,
		TmuxClient:     tmuxClient,
		Static:         static,
		DataDir:        cfg.DataDir,
		PublicHostname: publicHostname,
		ServerInfo: serviceserverinfo.New(
			hostinfo.New(),
			version.Version,
			cfg.DataDir,
			fileproject.WorkspaceRoot,
		),
		SelfUpdate: selfUpdateService,
		Files:      serviceworkspacefiles.New(hostfs.NewWorkspaceFileStore()),
		GitHistory: servicegithistory.New(gitcli.NewHistoryClient()),
		IDE:        serviceworkspaceide.New(codeServerBaseURL, fileproject.WorkspaceRoot),
	})
	if err != nil {
		log.Fatalf("init http handler: %v", err)
	}

	////////////////////////////////////////
	// Process runtime
	////////////////////////////////////////
	address := cfg.Addr()
	server := transport.NewHTTPServer(address, handler)
	startChatIndexWarmup(
		ctx,
		storeSet,
		configconstants.StartupChatIndexWarmupChatLimit,
		log.Default(),
	)
	log.Printf("remote.futrx listening on %s", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
