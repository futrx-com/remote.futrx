package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type agentApplicationStub struct {
	token, appID string
	request      applications.Request
	err          error
	calls        int
}

func (s *agentApplicationStub) CallAgentApplication(_ context.Context, token, id string, request applications.Request) (applications.Response, error) {
	s.calls++
	s.token = token
	s.appID = id
	s.request = request
	return applications.JSON(201, map[string]bool{"accepted": true}), s.err
}
func TestAgentApplicationsHTTPRequiresCapabilityAndForwardsGenericRequest(t *testing.T) {
	stub := &agentApplicationStub{}
	mux := http.NewServeMux()
	NewAgentApplicationsHandler(stub).RegisterRoutes(mux)
	request := httptest.NewRequest("POST", "/agent-api/applications/build-monitor/check?job=123", strings.NewReader(`{"command":"check"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 401 || stub.calls != 0 {
		t.Fatal("unauthenticated request reached app")
	}
	request.Header.Set("Authorization", "Bearer turn-grant")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 201 || stub.appID != "build-monitor" || stub.token != "turn-grant" || stub.request.Path != "check" || stub.request.QueryValue("job") != "123" || stub.request.Agent != nil {
		t.Fatalf("forwarding: %d %+v", response.Code, stub)
	}
	request = httptest.NewRequest("POST", "/agent-api/applications/build-monitor/check", strings.NewReader(strings.Repeat("x", (64<<10)+1)))
	request.Header.Set("Authorization", "Bearer turn-grant")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 400 || stub.calls != 1 {
		t.Fatal("oversized request reached app")
	}
	for _, check := range []struct {
		err    error
		status int
	}{{svc.ErrInvalidAgentGrant, 401}, {applications.ErrAgentAccess, 403}} {
		stub.err = check.err
		request = httptest.NewRequest("GET", "/agent-api/applications/build-monitor/check", nil)
		request.Header.Set("Authorization", "Bearer grant")
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != check.status {
			t.Fatalf("grant error status: %d", response.Code)
		}
	}
}
