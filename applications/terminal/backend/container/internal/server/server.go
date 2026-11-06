//go:build linux

// Package server answers terminal requests on the service's Unix socket. Only
// root in the container can reach the socket; Terminal's host backend reaches
// it through `remote-terminal bridge` after Remote has checked that the caller
// is a member of the project.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"futrx.local/catalog/applications/terminal/backend/container/internal/config"
	"futrx.local/catalog/applications/terminal/backend/container/internal/sessions"
)

type attachRequest struct {
	Session string `json:"session"`
	Cwd     string `json:"cwd"`
	Cols    int    `json:"cols"`
	Rows    int    `json:"rows"`
}

type attachResponse struct {
	Session    string `json:"session"`
	Generation string `json:"generation"`
}

type resizeRequest struct {
	Session    string `json:"session"`
	Generation string `json:"generation"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

func NewHandler(registry *sessions.Registry) http.Handler {
	h := handler{registry: registry}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /sessions", h.attach)
	mux.HandleFunc("GET /output", h.output)
	mux.HandleFunc("POST /input", h.input)
	mux.HandleFunc("POST /resize", h.resize)
	return mux
}

type handler struct {
	registry *sessions.Registry
}

func (h handler) attach(w http.ResponseWriter, r *http.Request) {
	var input attachRequest
	if json.NewDecoder(io.LimitReader(r.Body, config.MaxBodyBytes)).Decode(&input) != nil || !sessions.IDPattern.MatchString(input.Session) {
		sendError(w, http.StatusBadRequest, "Name a valid terminal session.")
		return
	}
	current, err := h.registry.Attach(input.Session, workingDirectory(input.Cwd), sessions.Size{Cols: input.Cols, Rows: input.Rows})
	if errors.Is(err, sessions.ErrTooManySessions) {
		sendError(w, http.StatusTooManyRequests, "Too many terminal sessions are running in this project.")
		return
	}
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("Could not start a shell: %v", err))
		return
	}
	sendJSON(w, http.StatusOK, attachResponse{
		Session:    current.ID,
		Generation: strconv.FormatUint(current.Generation, 10),
	})
}

func (h handler) output(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	current, ok := h.lookup(w, query.Get("session"), query.Get("generation"))
	if !ok {
		return
	}
	offset, err := strconv.ParseInt(query.Get("offset"), 10, 64)
	if err != nil {
		offset = -1
	}
	sendJSON(w, http.StatusOK, current.Read(r.Context(), offset))
}

func (h handler) input(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	current, ok := h.lookup(w, query.Get("session"), query.Get("generation"))
	if !ok {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, config.MaxBodyBytes))
	if err != nil {
		sendError(w, http.StatusBadRequest, "Unreadable input.")
		return
	}
	if err := current.Write(data); err != nil {
		sendEnded(w)
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h handler) resize(w http.ResponseWriter, r *http.Request) {
	var input resizeRequest
	if json.NewDecoder(io.LimitReader(r.Body, config.MaxBodyBytes)).Decode(&input) != nil {
		sendError(w, http.StatusBadRequest, "Send a terminal size.")
		return
	}
	current, ok := h.lookup(w, input.Session, input.Generation)
	if !ok {
		return
	}
	if err := current.Resize(sessions.Size{Cols: input.Cols, Rows: input.Rows}); errors.Is(err, sessions.ErrInvalidSize) {
		sendError(w, http.StatusBadRequest, "Invalid terminal size.")
		return
	} else if err != nil {
		sendEnded(w)
		return
	}
	sendJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// lookup resolves a viewer's session. A shell that has been replaced or
// collected answers 404 so the viewer opens the chat's shell again.
func (h handler) lookup(w http.ResponseWriter, id, generation string) (*sessions.Session, bool) {
	number, err := strconv.ParseUint(generation, 10, 64)
	if !sessions.IDPattern.MatchString(id) || err != nil {
		sendError(w, http.StatusBadRequest, "Name a valid terminal session.")
		return nil, false
	}
	current, ok := h.registry.Lookup(id, number)
	if !ok {
		sendEnded(w)
		return nil, false
	}
	return current, true
}

// workingDirectory passes on an absolute directory for the shell package to
// resolve inside the workspace; anything else starts in the workspace.
func workingDirectory(cwd string) string {
	if !filepath.IsAbs(cwd) || len(cwd) > config.MaxWorkingDirectoryBytes || strings.ContainsRune(cwd, 0) {
		return ""
	}
	return cwd
}

func sendEnded(w http.ResponseWriter) {
	sendError(w, http.StatusNotFound, "This terminal session has ended.")
}

func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}

func sendJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
