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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"futrx.local/catalog/applications/terminal/backend/container/internal/sessions"
)

const (
	// MaxBodyBytes matches what Remote forwards to a backend.
	MaxBodyBytes = 1 << 20
	// maxWorkingDirectoryBytes bounds the starting directory a browser names.
	maxWorkingDirectoryBytes = 4096
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

// Listen opens the service socket, replacing a stale one from an earlier run.
// Its directory and the socket itself are root-only.
func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// Serve answers requests on listener until it fails.
func Serve(listener net.Listener, manager *sessions.Manager) error {
	server := &http.Server{Handler: NewHandler(manager), ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}

func NewHandler(manager *sessions.Manager) http.Handler {
	h := handler{manager: manager}
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
	manager *sessions.Manager
}

func (h handler) attach(w http.ResponseWriter, r *http.Request) {
	var input attachRequest
	if json.NewDecoder(io.LimitReader(r.Body, MaxBodyBytes)).Decode(&input) != nil || !sessions.IDPattern.MatchString(input.Session) {
		sendError(w, http.StatusBadRequest, "Name a valid terminal session.")
		return
	}
	current, err := h.manager.Attach(input.Session, workingDirectory(input.Cwd), sessions.Size{Cols: input.Cols, Rows: input.Rows})
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
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes))
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
	if json.NewDecoder(io.LimitReader(r.Body, MaxBodyBytes)).Decode(&input) != nil {
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
	current, ok := h.manager.Lookup(id, number)
	if !ok {
		sendEnded(w)
		return nil, false
	}
	return current, true
}

// workingDirectory passes on an absolute directory for the shell package to
// resolve inside the workspace; anything else starts in the workspace.
func workingDirectory(cwd string) string {
	if !filepath.IsAbs(cwd) || len(cwd) > maxWorkingDirectoryBytes || strings.ContainsRune(cwd, 0) {
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
