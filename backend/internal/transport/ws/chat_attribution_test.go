package wstransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceprompt "github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	"github.com/gorilla/websocket"
)

type attributionAccess struct{}

func (attributionAccess) CallerAndAdmin(context.Context, *http.Request) (string, bool, error) {
	return "authenticated@example.com", false, nil
}
func (attributionAccess) HasAccess(context.Context, serviceproject.ID, string) (bool, error) {
	return true, nil
}

type attributionRunner struct{ inputs chan serviceprompt.StartInput }

func (r attributionRunner) Start(input serviceprompt.StartInput, _ func(servicechat.Event)) (serviceprompt.RunHandle, error) {
	r.inputs <- input
	return serviceprompt.RunHandle{}, nil
}
func (attributionRunner) CancelPrompt(servicechat.ID) bool { return false }
func (attributionRunner) RespondInteraction(servicechat.ID, agent.InteractionResponse) error {
	return nil
}

func TestChatPromptCannotSpoofItsRemoteAuthor(t *testing.T) {
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	meta, err := store.Create(context.Background(), servicechat.Meta{ID: "abcdef12", Title: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	runner := attributionRunner{inputs: make(chan serviceprompt.StartInput, 1)}
	socket := NewChatSocket(store, runhub.New(store), runner).WithAccessChecker(attributionAccess{})
	server := httptest.NewServer(socket.Handle(websocket.Upgrader{}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/chat/"+string(meta.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{"type": "prompt", "text": "hello", "userEmail": "spoofed@example.com", "actor": map[string]string{"email": "spoofed@example.com"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-runner.inputs:
		if input.Actor.Email != "authenticated@example.com" {
			t.Fatalf("client-controlled author: %#v", input.Actor)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt not received")
	}
}
