package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentcapability "github.com/futrx-com/remote.futrx.com/internal/service/agent/capability"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

type stubAgentCapabilitiesService struct {
	query agentcapability.ListQuery
}

func (s *stubAgentCapabilitiesService) List(
	_ context.Context,
	query agentcapability.ListQuery,
) ([]agent.Capabilities, error) {
	s.query = query
	return []agent.Capabilities{{
		Provider: agent.ProviderCodex,
		Label:    "Codex",
		Source:   agent.CapabilitySourceLive,
		Models:   []agent.ModelCapability{},
		Modes:    []agent.CapabilityOption{},
	}}, nil
}

func TestAgentCapabilitiesHandlerReturnsProjectCatalog(t *testing.T) {
	service := &stubAgentCapabilitiesService{}
	handler := NewAgentCapabilitiesHandler(service)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/agent-capabilities?projectId=project-1&refresh=1",
		nil,
	)
	request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()

	handler.HandleCollection(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if service.query.ProjectID != "project-1" || !service.query.Refresh || service.query.SessionCookie != "session-token" {
		t.Fatalf("unexpected query: %+v", service.query)
	}
	var body struct {
		Providers []agent.Capabilities `json:"providers"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Providers) != 1 || body.Providers[0].Provider != agent.ProviderCodex {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestAgentCapabilitiesHandlerRejectsNonGET(t *testing.T) {
	handler := NewAgentCapabilitiesHandler(&stubAgentCapabilitiesService{})
	response := httptest.NewRecorder()
	handler.HandleCollection(
		response,
		httptest.NewRequest(http.MethodPost, "/api/agent-capabilities", nil),
	)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
