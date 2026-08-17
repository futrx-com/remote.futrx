package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	servicetmux "github.com/futrx-com/remote.futrx.com/internal/service/tmux"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type TmuxHandler struct {
	sessions *servicetmux.Service
	audit    serviceaudit.Recorder
}

func NewTmuxHandler(sessions *servicetmux.Service) *TmuxHandler {
	return &TmuxHandler{sessions: sessions}
}

// WithAudit records host tmux session creation, deletion, and uploads. These
// run outside any project container, so they are host-level actions.
func (h *TmuxHandler) WithAudit(recorder serviceaudit.Recorder) *TmuxHandler {
	h.audit = recorder
	return h
}

func auditSessionTarget(name string) serviceaudit.Target {
	return serviceaudit.Target{Type: serviceaudit.TargetSession, ID: name, Name: name}
}

func (h *TmuxHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sessions", h.HandleSessionsCollection)
	mux.HandleFunc("/api/sessions/", h.HandleSessionResource)
}

func (h *TmuxHandler) HandleSessionsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httptransport.SendJSON(w, 200, h.sessions.List())
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil {
			httptransport.SendErr(w, 400, "invalid json")
			return
		}
		name, err := h.sessions.Create(body.Name)
		recordAudit(h.audit, r, serviceaudit.ActionWorkspaceTerminalOpen, auditSessionTarget(name), serviceaudit.Meta{"kind": "tmux"}, err)
		if err != nil {
			sendTmuxCreateError(w, err)
			return
		}
		httptransport.SendJSON(w, 201, map[string]string{"name": name})
	default:
		httptransport.SendErr(w, 405, "method not allowed")
	}
}

func (h *TmuxHandler) HandleSessionResource(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(rest, "/", 2)
	name := parts[0]
	if !servicetmux.ValidName(name) {
		httptransport.SendErr(w, 400, "invalid name")
		return
	}

	// /api/sessions/{name}/upload — multipart upload(s) into the session's cwd.
	if len(parts) == 2 && parts[1] == "upload" {
		if r.Method != http.MethodPost {
			httptransport.SendErr(w, 405, "method not allowed")
			return
		}
		cwd, err := h.sessions.UploadTarget(name)
		recordAudit(h.audit, r, serviceaudit.ActionWorkspaceFileUpload, auditSessionTarget(name), serviceaudit.Meta{"kind": "tmux"}, err)
		if err != nil {
			sendTmuxUploadError(w, err)
			return
		}
		httptransport.HandleMultipart(cwd, w, r)
		return
	}

	// /api/sessions/{name}/send — POST a chat message into the tmux session.
	if len(parts) == 2 && parts[1] == "send" {
		if r.Method != http.MethodPost {
			httptransport.SendErr(w, 405, "method not allowed")
			return
		}
		session, err := h.sessions.TextSession(name)
		if err != nil {
			if errors.Is(err, servicetmux.ErrSessionNotFound) {
				httptransport.SendErr(w, 404, "session not found")
			} else {
				httptransport.SendErr(w, 500, err.Error())
			}
			return
		}
		var body struct {
			Text       string `json:"text"`
			PressEnter *bool  `json:"pressEnter,omitempty"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
			httptransport.SendErr(w, 400, "invalid json")
			return
		}
		pressEnter := true
		if body.PressEnter != nil {
			pressEnter = *body.PressEnter
		}
		if err := session.SendText(body.Text, pressEnter); err != nil {
			httptransport.SendErr(w, 500, err.Error())
			return
		}
		httptransport.SendJSON(w, 200, map[string]bool{"ok": true})
		return
	}

	// /api/sessions/{name} — DELETE.
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			httptransport.SendErr(w, 405, "method not allowed")
			return
		}
		err := h.sessions.Delete(name)
		recordAudit(h.audit, r, serviceaudit.ActionWorkspaceTerminalClose, auditSessionTarget(name), serviceaudit.Meta{"kind": "tmux"}, err)
		if err != nil {
			if errors.Is(err, servicetmux.ErrSessionNotFound) {
				httptransport.SendErr(w, 404, "not found")
			} else {
				httptransport.SendErr(w, 500, err.Error())
			}
			return
		}
		httptransport.SendJSON(w, 200, map[string]bool{"ok": true})
		return
	}

	httptransport.SendErr(w, 404, "not found")
}

func sendTmuxCreateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servicetmux.ErrInvalidName):
		httptransport.SendErr(w, 400, "invalid name (alphanumeric, _ -, 1-32 chars)")
	case errors.Is(err, servicetmux.ErrSessionExists):
		httptransport.SendErr(w, 409, "session exists")
	default:
		httptransport.SendErr(w, 500, err.Error())
	}
}

func sendTmuxUploadError(w http.ResponseWriter, err error) {
	if errors.Is(err, servicetmux.ErrSessionNotFound) {
		httptransport.SendErr(w, 404, "session not found")
		return
	}
	httptransport.SendErr(w, 500, "could not resolve session cwd")
}
