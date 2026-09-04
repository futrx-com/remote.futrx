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

	remote "github.com/futrx-com/remote.futrx.com"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/config"
	"github.com/futrx-com/remote.futrx.com/internal/integration/gitcli"
	"github.com/futrx-com/remote.futrx.com/internal/integration/hostfs"
	"github.com/futrx-com/remote.futrx.com/internal/integration/hostinfo"
	"github.com/futrx-com/remote.futrx.com/internal/integration/lxc"
	"github.com/futrx-com/remote.futrx.com/internal/integration/tmuxcli"
	"github.com/futrx-com/remote.futrx.com/internal/integration/updatecli"
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
	// Prepare configuration
	ctx := context.Background()
	cfg := config.Load()
	if runCLICommand(ctx, cfg, os.Args) {
		return
	}
	publicHostname, err := config.PublicHostname(cfg.BaseURL)
	if err != nil {
		log.Fatalf("configure public hostname: %v", err)
	}

	// Register agent modules
	agentModules, err := config.NewAgentModules()
	if err != nil {
		log.Fatalf("configure agent modules: %v", err)
	}

	// Prepare container stack
	containerStack := config.NewContainerStack(
		lxc.New(),
		agentModules.Profiles(),
		config.ContainerStackOptions{
			AgentInstructions: provisioning.InstructionsTemplate(publicHostname),
		},
	)

	// Prepare stores
	storeSet, err := stores.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("init stores: %v", err)
	}
	maintenanceGuard := servicemaintenance.New(cfg.DataDir)
	selfUpdateService := serviceselfupdate.New(
		version.Version,
		cfg.InstallDir,
		cfg.DataDir,
		updatecli.New(),
	)

	// Register application services
	tmuxClient := tmuxcli.New()
	serviceSet, err := service.New(ctx, service.Dependencies{
		Chats:             storeSet.Chats,
		Projects:          storeSet.Projects,
		ProjectSecrets:    storeSet.ProjectSecrets,
		ProjectAccess:     storeSet.ProjectAccess,
		Schedules:         storeSet.Schedules,
		Auth:              storeSet.Auth,
		Users:             storeSet.Users,
		UserSettings:      storeSet.UserSettings,
		TwoFactor:         storeSet.TwoFactor,
		SessionRegistry:   storeSet.SessionRegistry,
		Notifications:     storeSet.Notifications,
		Push:              storeSet.Push,
		Usage:             storeSet.Usage,
		AuthBaseURL:       cfg.BaseURL,
		ProjectContainers: containerStack.ProjectDependencies(),
		AgentContainers:   containerStack.AgentDependencies(),
		AgentModules:      agentModules,
		AgentAPIKeys:      storeSet.AgentAPIKeys,
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
		PromptStartGate: maintenanceGuard,
	})
	if err != nil {
		log.Fatalf("init services: %v", err)
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

	// Prepare HTTP dependencies
	static, err := fs.Sub(remote.PublicFS, "public")
	if err != nil {
		log.Fatal(err)
	}
	codeServerBaseURL, err := config.CodeServerBaseURL(cfg.BaseURL)
	if err != nil {
		log.Fatalf("configure IDE URL: %v", err)
	}

	// Register HTTP transport
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

	// Start HTTP server
	address := cfg.Addr()
	server := transport.NewHTTPServer(address, handler)
	log.Printf("remote.futrx listening on %s", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
