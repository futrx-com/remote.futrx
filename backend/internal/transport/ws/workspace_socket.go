package wstransport

import (
	"context"
	"net/http"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"
	"github.com/gorilla/websocket"
)

type ChatLister interface {
	List(ctx context.Context) ([]servicechat.Meta, error)
}

type ProjectLister interface {
	List(ctx context.Context) ([]serviceproject.Meta, error)
}

// WorkspaceVisibility filters the initial snapshot so users only see chats
// + projects they can reach. Provided by the auth wiring; if nil, the
// snapshot is unfiltered (single-user dev mode).
type WorkspaceVisibility interface {
	CallerAndAdmin(ctx context.Context, r *http.Request) (string, bool, error)
	HasAccess(ctx context.Context, projectID serviceproject.ID, email string) (bool, error)
}

type WorkspaceSocket struct {
	chats      ChatLister
	projects   ProjectLister
	hub        *workspacehub.Hub
	visibility WorkspaceVisibility
}

type workspaceSnapshot struct {
	Type     string                `json:"type"`
	Chats    []servicechat.Meta    `json:"chats"`
	Projects []serviceproject.Meta `json:"projects"`
}

func NewWorkspaceSocket(chats ChatLister, projects ProjectLister, hub *workspacehub.Hub) *WorkspaceSocket {
	return &WorkspaceSocket{chats: chats, projects: projects, hub: hub}
}

func (s *WorkspaceSocket) WithVisibility(v WorkspaceVisibility) *WorkspaceSocket {
	s.visibility = v
	return s
}

func (s *WorkspaceSocket) RegisterRoutes(mux *http.ServeMux, upgrader websocket.Upgrader) {
	mux.HandleFunc("/ws/workspace", s.Handle(upgrader))
}

func (s *WorkspaceSocket) Handle(upgrader websocket.Upgrader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.handle(upgrader, w, r)
	}
}

func (s *WorkspaceSocket) handle(upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		http.Error(w, "workspace stream unavailable", http.StatusServiceUnavailable)
		return
	}

	email, isAdmin := "", true
	if s.visibility != nil {
		em, admin, err := s.visibility.CallerAndAdmin(r.Context(), r)
		if err != nil || em == "" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		email, isAdmin = em, admin
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 16)

	sub := s.hub.Subscribe()
	defer sub.Close()

	chats, err := s.chats.List(r.Context())
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
		return
	}
	projects, err := s.projects.List(r.Context())
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
		return
	}

	// Build the initial set of allowed project IDs for non-admin users.
	// This set is updated dynamically as project.upsert events arrive.
	needsFilter := s.visibility != nil && !isAdmin
	allowed := projectIDSet(projects)
	if needsFilter {
		projects = s.filterProjects(r.Context(), projects, email)
		allowed = projectIDSet(projects)
		chats = s.filterChats(chats, allowed)
	}

	done := make(chan struct{})
	go func() {
		defer conn.Close()
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteJSON(workspaceSnapshot{
			Type:     "workspace.snapshot",
			Chats:    chats,
			Projects: projects,
		}); err != nil {
			return
		}

		for {
			select {
			case ev, ok := <-sub.Events():
				if !ok {
					return
				}

				// Filter real-time events for non-admin users so
				// they only see projects and chats they have access
				// to. Admin users and single-user dev mode bypass
				// filtering entirely.
				if needsFilter {
					ev, ok = filterEvent(s.visibility, email, allowed, ev)
					if !ok {
						continue
					}
				}

				_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if err := conn.WriteJSON(ev); err != nil {
					return
				}
			case <-ticker.C:
				deadline := time.Now().Add(15 * time.Second)
				if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			close(done)
			return
		}
	}
}

// filterEvent decides whether ev should be forwarded to a non-admin client
// identified by email. It returns the (possibly transformed) event and true
// if the event should be sent, or a zero event and false to suppress it.
//
// The allowed map is maintained per-connection: project.upsert events that
// pass the access check add the project; project.delete events and failed
// access checks remove it. When an upsert arrives for a project the user
// previously had access to but no longer does, the event is rewritten as a
// project.delete so the client UI removes the stale entry.
func filterEvent(
	vis WorkspaceVisibility,
	email string,
	allowed map[serviceproject.ID]struct{},
	ev workspacehub.Event,
) (workspacehub.Event, bool) {
	switch ev.Type {
	case "project.upsert":
		if ev.Project == nil {
			return ev, false
		}
		// Use a short-lived background context: the original request
		// context may already be cancelled for long-lived connections.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ok, err := vis.HasAccess(ctx, ev.Project.ID, email)
		if err != nil || !ok {
			// If the user previously had access, they need a delete
			// event so the UI removes the now-inaccessible project.
			if _, had := allowed[ev.Project.ID]; had {
				delete(allowed, ev.Project.ID)
				return workspacehub.Event{
					Type: "project.delete",
					ID:   string(ev.Project.ID),
				}, true
			}
			return ev, false
		}
		allowed[ev.Project.ID] = struct{}{}
		return ev, true

	case "project.delete":
		pid := serviceproject.ID(ev.ID)
		delete(allowed, pid)
		// Only forward the delete if the user knew about this project.
		if _, had := allowed[pid]; had {
			return ev, true
		}
		// Still forward: the client might have it from an earlier
		// snapshot and a delete for an unknown ID is harmless.
		return ev, true

	case "chat.upsert":
		if ev.Chat == nil {
			return ev, false
		}
		// Loose chats (not bound to a project) are visible to everyone.
		if ev.Chat.ProjectID == "" {
			return ev, true
		}
		if _, ok := allowed[serviceproject.ID(ev.Chat.ProjectID)]; ok {
			return ev, true
		}
		return ev, false

	case "chat.delete":
		// chat.delete carries only an ID, not a project reference, so
		// we cannot filter it. Deleting an unknown chat on the client
		// is a harmless no-op, so we allow it through.
		return ev, true

	default:
		return ev, true
	}
}

func (s *WorkspaceSocket) filterProjects(ctx context.Context, in []serviceproject.Meta, email string) []serviceproject.Meta {
	out := make([]serviceproject.Meta, 0, len(in))
	for _, p := range in {
		ok, err := s.visibility.HasAccess(ctx, p.ID, email)
		if err == nil && ok {
			out = append(out, p)
		}
	}
	return out
}

func (s *WorkspaceSocket) filterChats(in []servicechat.Meta, allowedProjects map[serviceproject.ID]struct{}) []servicechat.Meta {
	out := make([]servicechat.Meta, 0, len(in))
	for _, c := range in {
		if c.ProjectID == "" {
			out = append(out, c)
			continue
		}
		if _, ok := allowedProjects[serviceproject.ID(c.ProjectID)]; ok {
			out = append(out, c)
		}
	}
	return out
}

func projectIDSet(projects []serviceproject.Meta) map[serviceproject.ID]struct{} {
	m := make(map[serviceproject.ID]struct{}, len(projects))
	for _, p := range projects {
		m[p.ID] = struct{}{}
	}
	return m
}
