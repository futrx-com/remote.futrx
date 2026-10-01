package httpmiddleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceaccess"
)

type WorkspaceChatGetter interface {
	Get(context.Context, chat.ID) (chat.Meta, error)
}

// Workspace guards transport-owned filesystem and PTY entry points after the
// authenticated middleware has attached the actor, before any I/O or upgrade.
type Workspace struct {
	Authorizer workspaceaccess.Authorizer
	Chats      WorkspaceChatGetter
}

func (m Workspace) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capability, id := workspaceTarget(r)
		if capability == "" {
			next.ServeHTTP(w, r)
			return
		}
		projectID := ""
		if id != "" {
			if m.Chats == nil {
				http.Error(w, "workspace unavailable", http.StatusServiceUnavailable)
				return
			}
			meta, err := m.Chats.Get(r.Context(), chat.ID(id))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			projectID = string(meta.ProjectID)
		}
		if err := workspaceaccess.Require(r.Context(), m.Authorizer, capability, projectID); err != nil {
			http.Error(w, "workspace access denied", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func workspaceTarget(r *http.Request) (string, string) {
	p := r.URL.Path
	if p == "/ws/terminal" {
		return "terminal", r.URL.Query().Get("chat")
	}
	if p == "/ws" || p == "/api/sessions" || strings.HasPrefix(p, "/api/sessions/") {
		return "terminal", ""
	}
	if strings.HasPrefix(p, "/api/chats/") {
		parts := strings.SplitN(strings.TrimPrefix(p, "/api/chats/"), "/", 2)
		if len(parts) == 2 {
			if parts[1] == "media-open" || parts[1] == "files" || strings.HasPrefix(parts[1], "files/") {
				return "files", parts[0]
			}
			if strings.HasPrefix(parts[1], "history/") {
				return "git", parts[0]
			}
		}
	}
	return "", ""
}
