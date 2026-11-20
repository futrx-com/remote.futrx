package wstransport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	harnessaccountaccess "github.com/futrx-com/remote.futrx.com/internal/service/harness/accountaccess"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	"github.com/gorilla/websocket"
)

// AgentAuthSocket exposes every catalog-registered agent status stream without
// depending on any concrete provider package.
type AgentAuthSocket struct {
	accountAccess *harnessaccountaccess.Service
	bindings      []harnessauth.Binding
}

func NewAgentAuthSocket(bindings []harnessauth.Binding) *AgentAuthSocket {
	return &AgentAuthSocket{bindings: append([]harnessauth.Binding(nil), bindings...)}
}

func (s *AgentAuthSocket) WithAccountAccess(access *harnessaccountaccess.Service) *AgentAuthSocket {
	s.accountAccess = access
	return s
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

func (s *AgentAuthSocket) handleSnapshot(binding harnessauth.Binding, upgrader websocket.Upgrader) http.HandlerFunc {
	return s.handleSubscription(binding, upgrader, binding.SubscribeSnapshots, true)
}

func (s *AgentAuthSocket) handle(binding harnessauth.Binding, upgrader websocket.Upgrader) http.HandlerFunc {
	return s.handleSubscription(binding, upgrader, binding.Subscribe, false)
}

func (s *AgentAuthSocket) handleSubscription(
	binding harnessauth.Binding,
	upgrader websocket.Upgrader,
	subscribe func() (harnessauth.Subscription, error),
	normalized bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !binding.Available() {
			http.Error(w, fmt.Sprintf("%s auth stream unavailable", binding.ID()), http.StatusServiceUnavailable)
			return
		}

		if s.accountAccess != nil && !normalized {
			if err := s.accountAccess.RequireManage(r.Context()); err != nil {
				http.Error(w, "permission denied", 403)
				return
			}
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

		streamCtx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go writeAgentAuthStatuses(streamCtx, conn, subscription, func(status any) (any, error) {
			if s.accountAccess == nil {
				if normalized {
					return binding.Snapshot(), nil
				}
				return binding.Status(), nil
			}
			if normalized {
				return s.accountAccess.Visible(streamCtx, binding.ID(), binding.Snapshot())
			}
			if err := s.accountAccess.RequireManage(streamCtx); err != nil {
				return nil, err
			}
			return binding.Status(), nil
		})

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}
}

func writeAgentAuthStatuses(ctx context.Context, conn *websocket.Conn, subscription harnessauth.Subscription, filter func(any) (any, error)) {
	defer conn.Close()
	for {
		tick, cancel := context.WithTimeout(ctx, 15*time.Second)
		status, ok := subscription.Next(tick)
		expired := tick.Err() == context.DeadlineExceeded
		cancel()
		if ctx.Err() != nil || (!ok && !expired) {
			return
		}
		var err error
		status, err = filter(status)
		if err != nil {
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteJSON(status); err != nil {
			return
		}
	}
}
