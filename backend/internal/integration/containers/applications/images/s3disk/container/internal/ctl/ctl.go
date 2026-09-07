// Package ctl exposes a per-mount control socket so `s3disk status` and
// `s3disk sync` can talk to a running mount without touching S3.
package ctl

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// SocketPath is where a mount's control socket lives inside its cache dir.
func SocketPath(cacheDir string) string { return filepath.Join(cacheDir, "control.sock") }

// Handler supplies the data the control socket serves.
type Handler struct {
	Status  func() any
	Sync    func(ctx context.Context) error
	Refresh func()
	Umount  func() error
}

// Server is a running control socket.
type Server struct {
	path string
	ln   net.Listener
	http *http.Server
}

// Serve starts the control socket, replacing any stale one left behind.
func Serve(path string, h Handler) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// A leftover socket from a killed mount would block bind.
	if _, err := os.Stat(path); err == nil {
		if c, derr := net.DialTimeout("unix", path, 200*time.Millisecond); derr == nil {
			c.Close()
			return nil, fmt.Errorf("another s3disk is already using %s", path)
		}
		_ = os.Remove(path)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, h.Status())
	})
	mux.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		if err := h.Sync(ctx); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]string{"result": "ok"})
	})
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		h.Refresh()
		writeJSON(w, map[string]string{"result": "ok"})
	})
	mux.HandleFunc("/umount", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"result": "unmounting"})
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = h.Umount()
		}()
	})

	s := &Server{path: path, ln: ln, http: &http.Server{Handler: mux}}
	go s.http.Serve(ln)
	return s, nil
}

// Close stops the control socket and removes it.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.http.Shutdown(ctx)
	_ = s.ln.Close()
	return os.Remove(s.path)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// Client talks to a mount's control socket.
type Client struct {
	http *http.Client
}

// NewClient dials the control socket at path.
func NewClient(path string) *Client {
	return &Client{http: &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", path)
			},
		},
	}}
}

// Get performs a control request and returns the raw JSON reply.
func (c *Client) Get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://s3disk"+endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := make([]byte, 0, 8192)
	tmp := make([]byte, 8192)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
