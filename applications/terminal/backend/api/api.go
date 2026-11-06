//go:build linux

// Package api serves Terminal's shell routes. Remote's project routes have
// already checked that the caller is a member of this installation's project;
// as with the shell built into Remote before, any member may use it. The
// shells themselves live in the container's remote-terminal service, which
// validates every request; this backend only relays them.
package api

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"futrx.local/catalog/applications/terminal/backend/config"
	"futrx.local/catalog/applications/terminal/backend/containerio"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Relay is the container bridge as the API uses it.
type Relay interface {
	Do(context.Context, containerio.Request) (containerio.Response, error)
}

type API struct {
	router    *applications.Router
	newBridge func(container string) Relay

	mu     sync.Mutex
	bridge Relay
}

var _ applications.Backend = (*API)(nil)

func New(newBridge func(container string) Relay) *API {
	b := &API{router: applications.NewRouter(), newBridge: newBridge}
	b.router.POST("sessions", "Open this chat's shell, starting it when needed", b.forward("/sessions"))
	b.router.GET("output", "Wait for shell output after an offset", b.forward("/output"))
	b.router.POST("input", "Type into the shell", b.forward("/input"))
	b.router.POST("resize", "Resize the shell's terminal", b.forward("/resize"))
	return b
}

func (b *API) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion, Routes: b.router.Routes()}, nil
}

func (b *API) Init(instance applications.Instance) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if instance.ContainerName != "" {
		b.bridge = b.newBridge(instance.ContainerName)
	}
	return nil
}

func (b *API) Handle(request applications.Request) (applications.Response, error) {
	return b.router.Serve(request), nil
}

func (b *API) forward(path string) applications.Handler {
	return func(request applications.Request) applications.Response {
		b.mu.Lock()
		bridge := b.bridge
		b.mu.Unlock()
		if bridge == nil {
			return applications.Errorf(http.StatusConflict, "This installation has no project container.")
		}
		ctx, cancel := context.WithTimeout(context.Background(), config.CallTimeout)
		defer cancel()
		response, err := bridge.Do(ctx, containerio.Request{
			Method: request.Method,
			Path:   path,
			Query:  url.Values(request.Query).Encode(),
			Body:   request.Body,
		})
		if err != nil {
			return applications.Errorf(http.StatusBadGateway, "Could not reach the terminal in the project container: %v", err)
		}
		return applications.Response{
			Status:  response.Status,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    response.Body,
		}
	}
}
