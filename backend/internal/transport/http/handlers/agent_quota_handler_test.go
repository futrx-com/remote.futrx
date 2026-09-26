package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

type stubAgentQuota struct {
	accounts  []agentquota.AccountView
	refreshes int
	views     int
}

func (s *stubAgentQuota) Refresh(context.Context) {
	s.refreshes++
}

func (s *stubAgentQuota) View() []agentquota.AccountView {
	if s.refreshes == 0 {
		panic("View before Refresh")
	}
	s.views++
	return s.accounts
}

func TestAgentQuotaResponseAndGuards(t *testing.T) {
	_, auth, _ := newClaimTestServer(t)
	token, err := auth.IssueSession(context.Background(), serviceauth.User{
		Email: "member@example.com", Sub: "google-member",
	}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	for _, test := range []struct {
		name    string
		method  string
		handler *AgentQuotaHandler
		cookie  string
		status  int
		body    string
	}{
		{"nil handler", http.MethodGet, nil, "", 200, `{"accounts":[]}`},
		{"nil service", http.MethodGet, NewAgentQuotaHandler(nil, nil), "", 200, `{"accounts":[]}`},
		{"method before availability", http.MethodPost, nil, "", 405, `{"error":"method not allowed"}`},
		{"no auth configured", http.MethodGet, NewAgentQuotaHandler(&stubAgentQuota{}, nil), "", 200, `{"accounts":[]}`},
		{"missing session", http.MethodGet, NewAgentQuotaHandler(&stubAgentQuota{}, auth), "", 401, `{"error":"authentication required"}`},
		{"invalid session", http.MethodGet, NewAgentQuotaHandler(&stubAgentQuota{}, auth), "invalid", 401, `{"error":"authentication required"}`},
		{"nil readings", http.MethodGet, NewAgentQuotaHandler(&stubAgentQuota{}, auth), token, 200, `{"accounts":[]}`},
		{"saved account, host login and failed read", http.MethodGet, NewAgentQuotaHandler(&stubAgentQuota{accounts: []agentquota.AccountView{
			{AccountQuota: agentquota.AccountQuota{Provider: "codex", Session: &agent.Quota{Window: agent.QuotaWindowSession, UsedPercent: &zero, MeasuredAt: 123}}},
			{AccountQuota: agentquota.AccountQuota{Provider: "codex", AccountID: "work", Weekly: &agent.Quota{Window: agent.QuotaWindowWeekly, Status: "allowed", MeasuredAt: 456}}},
			{AccountQuota: agentquota.AccountQuota{Provider: "codex", AccountID: "spare"}, Error: "sign-in expired"},
		}}, auth), token, 200, `{"accounts":[` +
			`{"provider":"codex","session":{"window":"session","usedPercent":0,"measuredAt":123}},` +
			`{"provider":"codex","accountId":"work","weekly":{"window":"weekly","status":"allowed","measuredAt":456}},` +
			`{"provider":"codex","accountId":"spare","error":"sign-in expired"}]}`},
		{"reported window without auth", http.MethodGet, NewAgentQuotaHandler(&stubAgentQuota{accounts: []agentquota.AccountView{{
			AccountQuota: agentquota.AccountQuota{Provider: "claude", AccountID: "work", Weekly: &agent.Quota{Window: agent.QuotaWindowWeekly, Status: "allowed", MeasuredAt: 456}},
		}}}, nil), "", 200, `{"accounts":[{"provider":"claude","accountId":"work","weekly":{"window":"weekly","status":"allowed","measuredAt":456}}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/agent-quota", nil)
			if test.cookie != "" {
				request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: test.cookie})
			}
			response := httptest.NewRecorder()
			test.handler.handle(response, request)
			if response.Code != test.status || response.Body.String() != test.body+"\n" {
				t.Fatalf("response = %d %s; want %d %s", response.Code, response.Body, test.status, test.body)
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("cache control = %q", response.Header().Get("Cache-Control"))
			}
			if test.handler != nil && test.handler.quota != nil {
				// Only a caller allowed to see the plans may make the service
				// ask the providers again.
				want := 0
				if test.status == http.StatusOK {
					want = 1
				}
				stub := test.handler.quota.(*stubAgentQuota)
				if stub.refreshes != want || stub.views != want {
					t.Fatalf("Refresh calls = %d, View calls = %d; want %d each", stub.refreshes, stub.views, want)
				}
			}
		})
	}
}
