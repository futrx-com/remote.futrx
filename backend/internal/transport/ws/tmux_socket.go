package wstransport

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/futrx-com/remote.futrx.com/internal/integration/tmuxcli"
	"github.com/gorilla/websocket"
)

type clientMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

type TmuxSessionClient interface {
	Has(name string) bool
	Create(name string) error
}

type TmuxSocket struct {
	client TmuxSessionClient
}

func NewTmuxSocket(client TmuxSessionClient) *TmuxSocket {
	return &TmuxSocket{client: client}
}

func (s *TmuxSocket) Handle(upgrader websocket.Upgrader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.handle(upgrader, w, r)
	}
}

func (s *TmuxSocket) RegisterRoutes(mux *http.ServeMux, upgrader websocket.Upgrader) {
	mux.HandleFunc("/ws", s.Handle(upgrader))
}

func (s *TmuxSocket) handle(upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("session")
	if !tmuxcli.ValidName(name) {
		http.Error(w, "invalid session name", http.StatusBadRequest)
		return
	}
	if !s.client.Has(name) {
		if err := s.client.Create(name); err != nil {
			http.Error(w, "create failed", http.StatusInternalServerError)
			return
		}
	}

	// -d kicks any other attached client so a phone reconnect takes over cleanly.
	cmd := exec.Command("tmux", "attach-session", "-d", "-t", name)
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

	// Single writer to ws (this goroutine reads from PTY, writes to ws).
	// gorilla/websocket forbids concurrent writes, so all WS writes happen here.
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
			if err != nil {
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	// Reader: WS -> PTY (+ control messages).
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
			var msg clientMsg
			if err := json.Unmarshal(data, &msg); err != nil {
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
