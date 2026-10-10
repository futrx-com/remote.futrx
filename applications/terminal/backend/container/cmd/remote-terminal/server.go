//go:build linux

package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed web/index.html web/terminal.js web/terminal.css web/reconnect.js web/vendor
var webFiles embed.FS

const (
	workspaceDirectory = "/workspace"
	detachedSessionTTL = 10 * time.Minute
	maxSessions        = 64
	pingInterval       = 25 * time.Second
	// Three missed pings: the browser answers each one, so silence this long
	// means the connection is gone even if TCP has not noticed.
	readTimeout = 80 * time.Second
)

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]+)?$`)

type clientMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func serve(port int) error {
	manager := newSessionManager(sessionConfig{
		workspace:   workspaceDirectory,
		shell:       []string{"/bin/bash", "-l"},
		env:         shellEnvironment(os.Getenv),
		detachedTTL: detachedSessionTTL,
		maxSessions: maxSessions,
	})
	server := &http.Server{
		// The Remote gateway reaches the service over the container network.
		Addr:              "0.0.0.0:" + strconv.Itoa(port),
		Handler:           newHandler(manager),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func shellEnvironment(getenv func(string) string) []string {
	path := getenv("PATH")
	if path == "" {
		path = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	return []string{
		"TERM=xterm-256color",
		"HOME=/root",
		"USER=root",
		"LOGNAME=root",
		"SHELL=/bin/bash",
		"LANG=C.UTF-8",
		"PATH=" + path,
	}
}

func newHandler(manager *sessionManager) http.Handler {
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(static)
	router := http.NewServeMux()
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	router.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveSession(manager, w, r)
	})
	router.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		setPageHeaders(w, r)
		files.ServeHTTP(w, r)
	}))
	return router
}

// setPageHeaders confines the page to its own origin and lets only the Remote
// shell that owns this application host embed it.
func setPageHeaders(w http.ResponseWriter, r *http.Request) {
	connect := "'self'"
	if hostPattern.MatchString(r.Host) {
		connect += " ws://" + r.Host + " wss://" + r.Host
	}
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src "+connect+
			"; frame-ancestors "+frameAncestor(r))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// frameAncestor names the Remote origin for an application host of the form
// <label>--<project>.<remote-host>. Anything else may only frame itself.
func frameAncestor(r *http.Request) string {
	label, parent, ok := strings.Cut(r.Host, ".")
	if !ok || !strings.Contains(label, "--") || !hostPattern.MatchString(parent) {
		return "'self'"
	}
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "http" {
		scheme = "http"
	}
	return scheme + "://" + parent
}

// sameOrigin rejects a browser on another origin opening a shell. Requests
// without an Origin header come from non-browser clients.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func serveSession(manager *sessionManager, w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request forbidden", http.StatusForbidden)
		return
	}
	id := r.URL.Query().Get("session")
	if !sessionIDPattern.MatchString(id) {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	viewer := newViewer(conn)
	current, err := manager.attach(id, r.URL.Query().Get("cwd"), viewer)
	if err != nil {
		reason := "terminal unavailable"
		if errors.Is(err, errTooManySessions) {
			reason = "too many terminal sessions"
		}
		viewer.finish(closeInternalError, reason)
		return
	}
	defer current.detach(viewer)

	stopPings := make(chan struct{})
	defer close(stopPings)
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if conn.ping() != nil {
					return
				}
			case <-stopPings:
				return
			}
		}
	}()

	for {
		opcode, payload, err := conn.readMessage(readTimeout)
		if err != nil {
			return
		}
		if opcode == opBinary {
			current.write(payload)
			continue
		}
		var message clientMessage
		if json.Unmarshal(payload, &message) != nil {
			continue
		}
		switch message.Type {
		case "input":
			current.write([]byte(message.Data))
		case "resize":
			current.resize(message.Cols, message.Rows)
		}
	}
}
