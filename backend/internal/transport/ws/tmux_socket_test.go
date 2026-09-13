package wstransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

type tmuxAccessStub struct {
	email string
	admin bool
	err   error
}

func (s tmuxAccessStub) CallerAndAdmin(context.Context, *http.Request) (string, bool, error) {
	return s.email, s.admin, s.err
}

type tmuxClientStub struct {
	hasName    string
	createName string
	createErr  error
}

func (s *tmuxClientStub) Has(name string) bool {
	s.hasName = name
	return false
}

func (s *tmuxClientStub) Create(name string) error {
	s.createName = name
	return s.createErr
}

func TestGlobalTerminalRequiresAuthentication(t *testing.T) {
	client := &tmuxClientStub{}
	socket := NewTmuxSocket(client).WithAccessChecker(tmuxAccessStub{})

	response := httptest.NewRecorder()
	socket.HandleGlobal(websocket.Upgrader{}).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/ws/host-terminal", nil),
	)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if client.hasName != "" || client.createName != "" {
		t.Fatal("terminal client was called before authentication")
	}
}

func TestGlobalTerminalRequiresAdministrator(t *testing.T) {
	client := &tmuxClientStub{}
	socket := NewTmuxSocket(client).WithAccessChecker(tmuxAccessStub{
		email: "member@example.com",
	})

	response := httptest.NewRecorder()
	socket.HandleGlobal(websocket.Upgrader{}).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/ws/host-terminal", nil),
	)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if client.hasName != "" || client.createName != "" {
		t.Fatal("terminal client was called for a non-administrator")
	}
}

func TestGlobalTerminalUsesPersistentFixedSession(t *testing.T) {
	client := &tmuxClientStub{createErr: errors.New("stop before starting tmux")}
	socket := NewTmuxSocket(client).WithAccessChecker(tmuxAccessStub{
		email: "admin@example.com",
		admin: true,
	})

	response := httptest.NewRecorder()
	socket.HandleGlobal(websocket.Upgrader{}).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/ws/host-terminal?session=ignored", nil),
	)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if client.hasName != globalTerminalSession || client.createName != globalTerminalSession {
		t.Fatalf("session calls = Has(%q), Create(%q), want %q", client.hasName, client.createName, globalTerminalSession)
	}
}

func TestLegacyHostTerminalAlsoRequiresAdministrator(t *testing.T) {
	client := &tmuxClientStub{}
	socket := NewTmuxSocket(client).WithAccessChecker(tmuxAccessStub{
		email: "member@example.com",
	})

	response := httptest.NewRecorder()
	socket.Handle(websocket.Upgrader{}).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/ws?session=existing", nil),
	)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
