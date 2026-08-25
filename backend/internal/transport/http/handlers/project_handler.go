package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type ProjectHandler struct {
	projects           *serviceproject.Service
	users              *serviceuser.Service
	auth               *serviceauth.Service
	apps               *ApplicationsHandler
	projectHostPattern *regexp.Regexp
	codeHostPattern    *regexp.Regexp
}

func NewProjectHandler(
	projects *serviceproject.Service,
	users *serviceuser.Service,
	auth *serviceauth.Service,
	apps *ApplicationsHandler,
	publicHostname string,
) *ProjectHandler {
	publicHostname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publicHostname)), ".")
	escapedHostname := regexp.QuoteMeta(publicHostname)
	return &ProjectHandler{
		projects: projects,
		users:    users,
		auth:     auth,
		apps:     apps,
		projectHostPattern: regexp.MustCompile(
			`^([a-z0-9][a-z0-9-]*)--(\d{4,5})\.dev\.` + escapedHostname + `$`,
		),
		codeHostPattern: regexp.MustCompile(
			`^([a-z0-9][a-z0-9-]*)\.code\.` + escapedHostname + `$`,
		),
	}
}

func (h *ProjectHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/projects", h.HandleCollection)
	mux.HandleFunc("/api/projects/reorder", h.HandleReorder)
	mux.HandleFunc("/api/projects/", h.HandleResource)
	mux.HandleFunc("/internal/tls-ask", h.HandleTLSAsk)
}

func (h *ProjectHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	email, isAdmin, err := h.caller(r)
	if err != nil {
		// Auth middleware already gated /api/* — if we still fail here the
		// session must have been torn down between checks. Treat as 401.
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		metas, err := h.projects.ListVisible(r.Context(), email, isAdmin)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		if metas == nil {
			metas = []serviceproject.Meta{}
		}
		httptransport.SendJSON(w, http.StatusOK, metas)

	case http.MethodPost:
		var body serviceproject.CreateInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		m, err := h.projects.Create(r.Context(), body, email)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusCreated, m)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ProjectHandler) HandleReorder(w http.ResponseWriter, r *http.Request) {
	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids := make([]serviceproject.ID, 0, len(body.IDs))
	for _, rawID := range body.IDs {
		id := serviceproject.ID(strings.TrimSpace(rawID))
		if !serviceproject.ValidID(id) {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid project id")
			return
		}
		allowed, err := h.allowed(r.Context(), id, email, isAdmin)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		if !allowed {
			httptransport.SendErr(w, http.StatusForbidden, "not a member of this project")
			return
		}
		ids = append(ids, id)
	}
	metas, err := h.projects.Reorder(r.Context(), ids)
	if err != nil {
		sendProjectError(w, err)
		return
	}
	if metas == nil {
		metas = []serviceproject.Meta{}
	}
	httptransport.SendJSON(w, http.StatusOK, metas)
}

func (h *ProjectHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.SplitN(rest, "/", 3)
	id := serviceproject.ID(parts[0])
	if id == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing id")
		return
	}

	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Every /api/projects/{id}/... route requires either admin OR project
	// membership. Resolve once and short-circuit.
	allowed, err := h.allowed(r.Context(), id, email, isAdmin)
	if err != nil {
		sendProjectError(w, err)
		return
	}
	if !allowed {
		httptransport.SendErr(w, http.StatusForbidden, "not a member of this project")
		return
	}

	if len(parts) >= 2 && parts[1] == "secrets" {
		h.handleSecrets(w, r, id, parts)
		return
	}

	if len(parts) >= 2 && parts[1] == "access" {
		h.handleAccess(w, r, id, parts, email, isAdmin)
		return
	}

	if len(parts) >= 2 && parts[1] == "agent-browser" {
		h.handleAgentBrowser(w, r, id, parts)
		return
	}

	if len(parts) >= 2 && parts[1] == "applications" {
		if h.apps == nil {
			httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
			return
		}
		h.apps.HandleProject(w, r, string(id), parts)
		return
	}

	if len(parts) >= 2 && parts[1] != "" {
		switch parts[1] {
		case "start":
			if r.Method != http.MethodPost {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			m, err := h.projects.Start(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, m)
		case "stop":
			if r.Method != http.MethodPost {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			m, err := h.projects.Stop(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, m)
		case "restart":
			if r.Method != http.MethodPost {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			m, err := h.projects.Restart(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, m)
		case "repair-network":
			if r.Method != http.MethodPost {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			info, err := h.projects.RepairNetwork(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, info)
		case "container":
			if r.Method != http.MethodGet {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			info, err := h.projects.InspectContainer(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, info)
		case "limits":
			if r.Method != http.MethodPut {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if !isAdmin {
				httptransport.SendErr(w, http.StatusForbidden, "admin only")
				return
			}
			var body serviceproject.ContainerLimits
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
				httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			info, err := h.projects.SetContainerLimits(r.Context(), id, body)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, info)
		case "apps":
			if r.Method != http.MethodGet {
				httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			apps, err := h.projects.ListContainerApps(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			if apps == nil {
				apps = []serviceproject.ContainerApp{}
			}
			httptransport.SendJSON(w, http.StatusOK, apps)
		default:
			httptransport.SendErr(w, http.StatusNotFound, "unknown action")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		m, err := h.projects.Get(r.Context(), id)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, m)

	case http.MethodPatch:
		var body serviceproject.UpdateInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		m, err := h.projects.Update(r.Context(), id, body)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, m)

	case http.MethodDelete:
		if !isAdmin {
			httptransport.SendErr(w, http.StatusForbidden, "admin only")
			return
		}
		if err := h.projects.Delete(r.Context(), id); err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type agentBrowserResponse struct {
	Status       serviceproject.AgentBrowserStatus `json:"status"`
	URL          string                            `json:"url"`
	Slug         string                            `json:"slug"`
	Port         int                               `json:"port"`
	Error        string                            `json:"error,omitempty"`
	Core         string                            `json:"core,omitempty"`
	View         string                            `json:"view,omitempty"`
	ViewerCount  int                               `json:"viewerCount,omitempty"`
	UptimeSec    int64                             `json:"uptimeSec,omitempty"`
	LastActivity int64                             `json:"lastActivity,omitempty"`
}

func (h *ProjectHandler) handleAgentBrowser(w http.ResponseWriter, r *http.Request, id serviceproject.ID, parts []string) {
	action := ""
	if len(parts) >= 3 {
		action = strings.Trim(parts[2], "/")
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		info, err := h.projects.AgentBrowserStatus(r.Context(), id)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		h.sendAgentBrowserInfo(w, r, info)

	case r.Method == http.MethodPost && action == "start":
		info, err := h.projects.StartAgentBrowser(r.Context(), id)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		h.sendAgentBrowserInfo(w, r, info)

	case r.Method == http.MethodDelete && action == "":
		scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
		if scope == "view" {
			if err := h.projects.StopAgentBrowserView(r.Context(), id); err != nil {
				sendProjectError(w, err)
				return
			}
			info, err := h.projects.AgentBrowserStatus(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			h.sendAgentBrowserInfo(w, r, info)
			return
		}
		if err := h.projects.StopAgentBrowser(r.Context(), id); err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]serviceproject.AgentBrowserStatus{
			"status": serviceproject.AgentBrowserStatusStopped,
		})

	case action == "":
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")

	default:
		httptransport.SendErr(w, http.StatusNotFound, "unknown action")
	}
}

func (h *ProjectHandler) sendAgentBrowserInfo(w http.ResponseWriter, r *http.Request, info serviceproject.AgentBrowserInfo) {
	resp := agentBrowserResponse{
		Status:       info.Status,
		Slug:         info.Slug,
		Port:         info.Port,
		Error:        info.Error,
		Core:         info.Core,
		View:         info.View,
		ViewerCount:  info.ViewerCount,
		UptimeSec:    info.UptimeSec,
		LastActivity: info.LastActivity,
	}
	if info.Status == serviceproject.AgentBrowserStatusReady && info.Slug != "" && info.Port != 0 {
		resp.URL = buildAgentBrowserURL(r, info.Slug, info.Port)
	}
	httptransport.SendJSON(w, http.StatusOK, resp)
}

func buildAgentBrowserURL(r *http.Request, slug string, port int) string {
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.TrimPrefix(host, "code.")
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
			scheme = "http"
		} else {
			scheme = "https"
		}
	}
	return fmt.Sprintf("%s://%s--%d.dev.%s/vnc.html?autoconnect=1&resize=scale&reconnect=1", scheme, slug, port, host)
}

// HandleTLSAsk lets Caddy issue on-demand certificates only for preview and
// code subdomains belonging to projects that currently exist.
func (h *ProjectHandler) HandleTLSAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if domain == "" {
		http.Error(w, "missing domain", http.StatusBadRequest)
		return
	}
	var slug string
	if mm := h.projectHostPattern.FindStringSubmatch(domain); mm != nil {
		slug = mm[1]
		port, err := strconv.Atoi(mm[2])
		if err != nil || port < 1024 || port > 65535 {
			http.Error(w, "port out of range", http.StatusNotFound)
			return
		}
	} else if mm := h.codeHostPattern.FindStringSubmatch(domain); mm != nil {
		slug = mm[1]
	} else {
		http.Error(w, "host not a recognized project domain", http.StatusNotFound)
		return
	}

	if _, err := h.projects.GetBySlug(r.Context(), slug); err != nil {
		http.Error(w, "no such project", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ProjectHandler) handleSecrets(w http.ResponseWriter, r *http.Request, id serviceproject.ID, parts []string) {
	// /api/projects/{id}/secrets[/{key}]
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		secrets, err := h.projects.ListSecrets(r.Context(), id)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		if secrets == nil {
			secrets = []serviceproject.Secret{}
		}
		httptransport.SendJSON(w, http.StatusOK, secrets)
		return
	}

	key := parts[2]
	if key == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing secret key")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		s, err := h.projects.SetSecret(r.Context(), id, key, body.Value)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, s)
	case http.MethodDelete:
		if err := h.projects.DeleteSecret(r.Context(), id, key); err != nil {
			sendProjectError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAccess handles /api/projects/{id}/access[/{email}]. Any project
// member (admin or not) may list and edit the access list, with a guardrail:
// non-admins may not remove the last member of a project (which would
// orphan it from everyone but admins).
func (h *ProjectHandler) handleAccess(
	w http.ResponseWriter,
	r *http.Request,
	id serviceproject.ID,
	parts []string,
	callerEmail string,
	isAdmin bool,
) {
	// /api/projects/{id}/access
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			list, err := h.projects.ListAccess(r.Context(), id)
			if err != nil {
				sendProjectError(w, err)
				return
			}
			if list == nil {
				list = []string{}
			}
			httptransport.SendJSON(w, http.StatusOK, list)
			return
		case http.MethodPost:
			var body struct {
				Email string `json:"email"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
				httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			email := strings.ToLower(strings.TrimSpace(body.Email))
			if email == "" {
				httptransport.SendErr(w, http.StatusBadRequest, "missing email")
				return
			}
			// Only registered users can be added to a project's access list.
			if h.users != nil {
				registered, err := h.users.IsRegistered(r.Context(), email)
				if err != nil {
					sendProjectError(w, err)
					return
				}
				if !registered {
					httptransport.SendErr(w, http.StatusNotFound, "user not registered - add them in the users panel first")
					return
				}
			}
			if err := h.projects.AddAccess(r.Context(), id, email); err != nil {
				sendProjectError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, map[string]string{"email": email})
			return
		default:
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	// /api/projects/{id}/access/{email}
	if r.Method != http.MethodDelete {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	emailRaw := parts[2]
	if emailRaw == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing email")
		return
	}
	email, err := url.PathUnescape(emailRaw)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid email path")
		return
	}
	email = strings.ToLower(strings.TrimSpace(email))

	// Guardrail: non-admins can't remove the last member.
	if !isAdmin {
		members, err := h.projects.ListAccess(r.Context(), id)
		if err != nil {
			sendProjectError(w, err)
			return
		}
		if len(members) <= 1 {
			httptransport.SendErr(w, http.StatusConflict, "cannot remove the last member - ask an admin")
			return
		}
	}
	if err := h.projects.RemoveAccess(r.Context(), id, email); err != nil {
		sendProjectError(w, err)
		return
	}
	_ = callerEmail // available for future audit logging
	httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// caller is a thin wrapper around callerStateFromRequest local to this
// handler, since the auth field is required for every route here.
func (h *ProjectHandler) caller(r *http.Request) (string, bool, error) {
	if h.auth == nil {
		return "", true, nil
	}
	return callerStateFromRequest(r.Context(), r, h.auth)
}

// allowed returns true if the caller can act on this project at all. Admins
// are always allowed. Members are allowed when their email is in the
// project's access list.
func (h *ProjectHandler) allowed(ctx context.Context, id serviceproject.ID, email string, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	// Pre-flight: project must exist (so we return 404 vs 403 correctly).
	if _, err := h.projects.Get(ctx, id); err != nil {
		return false, err
	}
	if email == "" {
		return false, nil
	}
	return h.projects.HasAccess(ctx, id, email)
}

func sendProjectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceproject.ErrNameRequired),
		errors.Is(err, serviceproject.ErrInvalidID),
		errors.Is(err, serviceproject.ErrInvalidSecretKey),
		errors.Is(err, serviceproject.ErrInvalidLimits):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceproject.ErrSecretsUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, serviceproject.ErrNameAlreadyExists):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceproject.ErrNotFound):
		httptransport.SendErr(w, http.StatusNotFound, "project not found")
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
