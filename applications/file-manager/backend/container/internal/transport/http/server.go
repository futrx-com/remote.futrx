package httptransport

import (
	"net/http"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
)

type RouteRegistrar interface {
	RegisterRoutes(*http.ServeMux)
}

type Middleware interface {
	Wrap(http.Handler) http.Handler
}

type Handlers struct {
	Files      RouteRegistrar
	Static     http.Handler
	Middleware Middleware
}

func NewHandler(handlers Handlers) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("file-manager ready")) })
	if handlers.Files != nil {
		handlers.Files.RegisterRoutes(mux)
	}
	if handlers.Static != nil {
		mux.Handle("GET /", handlers.Static)
	}
	var handler http.Handler = mux
	if handlers.Middleware != nil {
		handler = handlers.Middleware.Wrap(handler)
	}
	return handler
}

func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		IdleTimeout:       config.IdleTimeout,
	}
}
