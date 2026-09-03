package wstransport

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/creack/pty"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/gorilla/websocket"
)

type TerminalChatGetter interface {
	Get(ctx context.Context, id servicechat.ID) (servicechat.Meta, error)
}

type TerminalProjectStarter interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	Start(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
}

type ContainerTerminalSocket struct {
	chats    TerminalChatGetter
	projects TerminalProjectStarter
	access   ProjectAccessChecker
}

func NewContainerTerminalSocket(
	chats TerminalChatGetter,
	projects TerminalProjectStarter,
) *ContainerTerminalSocket {
	return &ContainerTerminalSocket{chats: chats, projects: projects}
}

func (s *ContainerTerminalSocket) WithAccessChecker(access ProjectAccessChecker) *ContainerTerminalSocket {
	s.access = access
	return s
}

func (s *ContainerTerminalSocket) RegisterRoutes(mux *http.ServeMux, upgrader websocket.Upgrader) {
	mux.HandleFunc("/ws/terminal", s.Handle(upgrader))
}

func (s *ContainerTerminalSocket) Handle(upgrader websocket.Upgrader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.handle(upgrader, w, r)
	}
}

func (s *ContainerTerminalSocket) handle(upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	chatID := servicechat.ID(strings.TrimSpace(r.URL.Query().Get("chat")))
	if !servicechat.ValidID(chatID) {
		http.Error(w, "invalid chat id", http.StatusBadRequest)
		return
	}
	if s.chats == nil || s.projects == nil {
		http.Error(w, "terminal unavailable", http.StatusServiceUnavailable)
		return
	}

	meta, err := s.chats.Get(r.Context(), chatID)
	if err != nil {
		http.Error(w, "chat not found", http.StatusNotFound)
		return
	}
	if meta.ProjectID == "" {
		http.Error(w, "chat has no project container", http.StatusBadRequest)
		return
	}

	if s.access != nil {
		email, isAdmin, err := s.access.CallerAndAdmin(r.Context(), r)
		if err != nil || email == "" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !isAdmin {
			ok, err := s.access.HasAccess(r.Context(), serviceproject.ID(meta.ProjectID), email)
			if err != nil {
				http.Error(w, "access check failed", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "not a member of this project", http.StatusForbidden)
				return
			}
		}
	}

	project, err := s.projects.Get(r.Context(), serviceproject.ID(meta.ProjectID))
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if project.ContainerName == "" {
		http.Error(w, "project has no container", http.StatusBadRequest)
		return
	}
	if project.Status != serviceproject.StatusRunning {
		project, err = s.projects.Start(r.Context(), project.ID)
		if err != nil {
			http.Error(w, "start container failed", http.StatusInternalServerError)
			return
		}
	}

	cwd := containerCwd(meta.Cwd, project.Cwd)
	args := []string{
		"exec",
		"--env", "TERM=xterm-256color",
		"--env", "HOME=/root",
		"--env", "REMOTE_CWD=" + cwd,
		project.ContainerName,
		"--",
		"bash",
		"-lc",
		"cd \"$REMOTE_CWD\" 2>/dev/null || cd /workspace; exec bash -l",
	}
	cmd := exec.CommandContext(r.Context(), "lxc", args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		http.Error(w, "pty failed", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cancel()
			_ = ptmx.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
			_ = conn.Close()
		})
	}
	defer cleanup()

	go func() {
		defer cleanup()
		buf := make([]byte, 8192)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					return
				}
			}
			if err != nil || ctx.Err() != nil {
				return
			}
		}
	}()

	conn.SetReadLimit(1 << 20)
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			msg, ok := parseTerminalClientMessage(data)
			if !ok {
				continue
			}
			switch msg.Type {
			case "input":
				if _, err := ptmx.Write([]byte(msg.Data)); err != nil {
					return
				}
			case "resize":
				_ = pty.Setsize(ptmx, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
			}
		}
	}
}

func containerCwd(chatCwd, projectCwd string) string {
	if strings.TrimSpace(chatCwd) == "" || strings.TrimSpace(projectCwd) == "" {
		return "/workspace"
	}

	rel, err := filepath.Rel(filepath.Clean(projectCwd), filepath.Clean(chatCwd))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "/workspace"
	}
	return path.Join("/workspace", filepath.ToSlash(rel))
}

func parseTerminalClientMessage(data []byte) (clientMsg, bool) {
	var msg clientMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return clientMsg{}, false
	}
	return msg, true
}
