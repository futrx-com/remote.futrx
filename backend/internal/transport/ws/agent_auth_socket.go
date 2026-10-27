package wstransport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/gorilla/websocket"
)

// AgentAuthSocket exposes every catalog-registered agent status stream without
// depending on any concrete provider package.
type AgentAuthSocket struct {
	bindings []agentauth.Binding
}

func NewAgentAuthSocket(bindings []agentauth.Binding) *AgentAuthSocket {
	return &AgentAuthSocket{bindings: append([]agentauth.Binding(nil), bindings...)}
}

func (s *AgentAuthSocket) RegisterRoutes(mux *http.ServeMux, upgrader websocket.Upgrader) {
	for _, binding := range s.bindings {
		binding := binding
		mux.HandleFunc("/ws/"+string(binding.ID())+"/auth-status", s.handle(binding, upgrader))
		if binding.Available() {
			mux.HandleFunc("/ws/agent-auth/"+string(binding.ID()), s.handleSnapshot(binding, upgrader))
		}
	}
}

func (s *AgentAuthSocket) handleSnapshot(binding agentauth.Binding, upgrader websocket.Upgrader) http.HandlerFunc {
	return s.handleSubscription(binding, upgrader, binding.SubscribeSnapshots)
}

func (s *AgentAuthSocket) handle(binding agentauth.Binding, upgrader websocket.Upgrader) http.HandlerFunc {
	return s.handleSubscription(binding, upgrader, binding.Subscribe)
}

func (s *AgentAuthSocket) handleSubscription(
	binding agentauth.Binding,
	upgrader websocket.Upgrader,
	subscribe func() (agentauth.Subscription, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !binding.Available() {
			http.Error(w, fmt.Sprintf("%s auth stream unavailable", binding.ID()), http.StatusServiceUnavailable)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadLimit(1024)

		subscription, err := subscribe()
		if err != nil {
			return
		}
		defer subscription.Close()

		streamCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go writeAgentAuthStatuses(streamCtx, conn, subscription)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}
}

func writeAgentAuthStatuses(ctx context.Context, conn *websocket.Conn, subscription agentauth.Subscription) {
	defer conn.Close()
	for {
		status, ok := subscription.Next(ctx)
		if !ok {
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteJSON(status); err != nil {
			return
		}
	}
}
