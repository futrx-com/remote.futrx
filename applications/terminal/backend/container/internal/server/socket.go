//go:build linux

package server

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"futrx.local/catalog/applications/terminal/backend/container/internal/config"
	"futrx.local/catalog/applications/terminal/backend/container/internal/sessions"
)

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
func Serve(listener net.Listener, registry *sessions.Registry) error {
	server := &http.Server{Handler: NewHandler(registry), ReadHeaderTimeout: config.ReadHeaderTimeout}
	return server.Serve(listener)
}
