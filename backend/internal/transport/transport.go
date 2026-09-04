package transport

import (
	"context"
	"io/fs"
	"net/http"

	service "github.com/futrx-com/remote.futrx.com/internal/service"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicegithistory "github.com/futrx-com/remote.futrx.com/internal/service/githistory"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceselfupdate "github.com/futrx-com/remote.futrx.com/internal/service/selfupdate"
	serviceserverinfo "github.com/futrx-com/remote.futrx.com/internal/service/serverinfo"
	serviceworkspacefiles "github.com/futrx-com/remote.futrx.com/internal/service/workspacefiles"
	serviceworkspaceide "github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	httphandlers "github.com/futrx-com/remote.futrx.com/internal/transport/http/handlers"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
	wstransport "github.com/futrx-com/remote.futrx.com/internal/transport/ws"
)

type TmuxClient interface {
	wstransport.TmuxSessionClient
}

type Dependencies struct {
	Services       service.Services
	TmuxClient     TmuxClient
	Static         fs.FS
	DataDir        string
	PublicHostname string
	ServerInfo     *serviceserverinfo.Service
	SelfUpdate     *serviceselfupdate.Service
	Files          *serviceworkspacefiles.Service
	GitHistory     *servicegithistory.Service
	IDE            *serviceworkspaceide.Service
}

func NewHTTPHandler(deps Dependencies) (http.Handler, error) {
	agentAuthBindings := deps.Services.Agents.Bindings()
	var auth httptransport.RouteRegistrar
	var middleware httptransport.Middleware
	if deps.Services.Auth != nil {
		auth = httphandlers.NewAuthHandler(deps.Services.Auth, deps.Services.Access)
		providerAuthPrefixes := make([]string, 0, len(agentAuthBindings)*2)
		for _, binding := range agentAuthBindings {
			provider := string(binding.ID())
			providerAuthPrefixes = append(
				providerAuthPrefixes,
				"/api/"+provider+"/",
				"/ws/"+provider+"/auth-status",
			)
		}
		providerAuthPrefixes = append(providerAuthPrefixes, "/api/agent-auth", "/ws/agent-auth/")
		middleware = httpmiddleware.NewAuth(deps.Services.Auth).
			RequireLocalAdminSetup(deps.Services.Auth.LocalAdminConfigured).
			RequireProviderLogin(
				deps.Services.Agents.AccessReady,
				providerAuthPrefixes...,
			)
	}

	gate := newAccessGate(deps.Services.Auth, deps.Services.Projects, deps.Services.Chats)

	uploads, err := httptransport.NewUploadHandlerWithGate(deps.Services.Chats, deps.DataDir, gate)
	if err != nil {
		return nil, err
	}
	chatSocket := wstransport.NewChatSocket(deps.Services.Chats, deps.Services.Runs, deps.Services.Prompt)
	terminalSocket := wstransport.NewContainerTerminalSocket(deps.Services.Chats, deps.Services.Projects)
	workspaceSocket := wstransport.NewWorkspaceSocket(
		deps.Services.Chats,
		deps.Services.Projects,
		deps.Services.Workspace,
	)
	if deps.Services.Auth != nil {
		chatSocket = chatSocket.WithAccessChecker(gate)
		terminalSocket = terminalSocket.WithAccessChecker(gate)
		workspaceSocket = workspaceSocket.WithVisibility(gate)
	}
	scheduleHandler := httphandlers.NewScheduleHandler(
		deps.Services.Schedules,
		deps.Services.ScheduleCaps,
		deps.Services.Auth,
	)
	usageHandler := httphandlers.NewUsageHandler(deps.Services.Usage, deps.Services.Auth)
	chatHandler := httphandlers.NewChatHandler(
		deps.Services.Chats,
		deps.Services.ChatAccess,
		deps.Services.Auth,
		deps.Files,
		deps.GitHistory,
		deps.IDE,
	).WithSchedules(scheduleHandler)

	return httptransport.NewHandler(httptransport.Handlers{
		Sessions: httphandlers.NewTmuxHandler(deps.Services.Tmux),
		Chats:    chatHandler,
		Projects: httphandlers.NewProjectHandler(
			deps.Services.Projects,
			deps.Services.Users,
			deps.Services.Auth,
			deps.PublicHostname,
		).WithUsage(usageHandler),
		Users: httphandlers.NewUsersHandler(deps.Services.Users, deps.Services.Auth),
		AgentAuth: httphandlers.NewAgentAuthHandler(
			agentAuthBindings,
			deps.Services.Auth,
			deps.Services.Agents,
		),
		AgentCapabilities: httphandlers.NewAgentCapabilitiesHandler(deps.Services.AgentCapabilities),
		UserSettings: httphandlers.NewUserSettingsHandler(
			deps.Services.UserSettings,
			deps.Services.Auth,
		),
		Security: httphandlers.NewSecurityHandler(deps.Services.Auth),
		Push: httphandlers.NewPushHandler(
			deps.Services.Push,
			deps.Services.Auth,
			deps.Services.Presence,
		),
		ServerInfo:       httphandlers.NewServerInfoHandler(deps.ServerInfo),
		SelfUpdate:       httphandlers.NewSelfUpdateHandler(deps.SelfUpdate, deps.Services.Auth),
		Skills:           httphandlers.NewSkillHandler(deps.Services.Skills),
		BrowserInspector: httphandlers.NewBrowserInspectorHandler(),
		Schedules:        scheduleHandler,
		Usage:            usageHandler,
		Uploads:          uploads,
		TmuxWS:           wstransport.NewTmuxSocket(deps.TmuxClient),
		TerminalWS:       terminalSocket,
		ChatWS:           chatSocket,
		WorkspaceWS:      workspaceSocket,
		AgentAuthWS:      wstransport.NewAgentAuthSocket(agentAuthBindings),
		Auth:             auth,
		Middleware:       middleware,
		Static:           httptransport.NewStaticHandler(deps.Static),
	}), nil
}

func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return httptransport.NewServer(addr, handler)
}

// accessGate is the small adapter that satisfies the per-WS / per-upload
// gating interfaces. It centralizes "who is the caller?" and "can they
// reach this project?" lookups so each socket / upload site doesn't have
// to wire up the auth + project + chat services itself.
type accessGate struct {
	auth      *serviceauth.Service
	principal *httptransport.PrincipalResolver
	projects  *serviceproject.Service
	chats     chatProjectFinder
}

type chatProjectFinder interface {
	Get(ctx context.Context, id servicechat.ID) (servicechat.Meta, error)
}

func newAccessGate(auth *serviceauth.Service, projects *serviceproject.Service, chats chatProjectFinder) *accessGate {
	if auth == nil {
		return nil
	}
	return &accessGate{
		auth: auth, principal: httptransport.NewPrincipalResolver(auth),
		projects: projects, chats: chats,
	}
}

func (g *accessGate) CallerAndAdmin(ctx context.Context, r *http.Request) (string, bool, error) {
	if g == nil || g.auth == nil {
		return "", true, nil
	}
	session, err := g.principal.Session(r)
	if err != nil || session == nil {
		return "", false, err
	}
	isAdmin, _ := g.auth.IsAdmin(ctx, session.Email)
	return session.Email, isAdmin, nil
}

func (g *accessGate) HasAccess(ctx context.Context, projectID serviceproject.ID, email string) (bool, error) {
	if g == nil || g.projects == nil {
		return false, nil
	}
	return g.projects.HasAccess(ctx, projectID, email)
}

func (g *accessGate) CanUploadToChat(ctx context.Context, r *http.Request, chatID servicechat.ID) (bool, error) {
	if g == nil {
		return true, nil
	}
	email, isAdmin, err := g.CallerAndAdmin(ctx, r)
	if err != nil || email == "" {
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	if g.chats == nil {
		return false, nil
	}
	meta, err := g.chats.Get(ctx, chatID)
	if err != nil {
		return false, err
	}
	if meta.ProjectID == "" {
		return true, nil
	}
	return g.HasAccess(ctx, serviceproject.ID(meta.ProjectID), email)
}
