package minimax

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

func TestAPIKeyValidatorAcceptsAuthenticatedTokenPlan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/token_plan/remains" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-cp-valid-key" {
			t.Errorf("Authorization = %q", got)
			return
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model_remains":[],"base_resp":{"status_code":0,"status_msg":"success"}}`))
	}))
	defer server.Close()

	validator := &apiKeyValidator{client: server.Client(), endpoint: server.URL + "/v1/token_plan/remains"}
	if err := validator.ValidateAPIKey(context.Background(), "sk-cp-valid-key"); err != nil {
		t.Fatal(err)
	}
}

func TestAPIKeyValidatorRejectsPayAsYouGoKeysWithoutCallingMiniMax(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	validator := &apiKeyValidator{client: server.Client(), endpoint: server.URL}
	err := validator.ValidateAPIKey(context.Background(), "standard-pay-as-you-go-key")
	if !errors.Is(err, agentauth.ErrAPIKeyRejected) || !errors.Is(err, ErrTokenPlanKeyRequired) {
		t.Fatalf("error = %v, want Token Plan rejection", err)
	}
	if !strings.Contains(err.Error(), "pay-as-you-go API keys are not supported") {
		t.Fatalf("error = %q, want subscription guidance", err)
	}
	if called {
		t.Fatal("standard key reached MiniMax validation endpoint")
	}
}

func TestAPIKeyValidatorRejectsUnauthorizedKeysWithoutEchoingProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"body-must-not-escape"}`))
	}))
	defer server.Close()

	validator := &apiKeyValidator{client: server.Client(), endpoint: server.URL}
	err := validator.ValidateAPIKey(context.Background(), "sk-cp-rejected-key")
	if !errors.Is(err, agentauth.ErrAPIKeyRejected) {
		t.Fatalf("error = %v, want ErrAPIKeyRejected", err)
	}
	if strings.Contains(err.Error(), "body-must-not-escape") {
		t.Fatalf("provider body escaped into error: %v", err)
	}
}

func TestAPIKeyValidatorTreatsProviderAndPayloadFailuresAsTemporary(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "provider error", status: http.StatusInternalServerError, body: `{"error":"internal"}`},
		{name: "malformed JSON", status: http.StatusOK, body: `{`},
		{name: "missing status", status: http.StatusOK, body: `{"model_remains":[]}`},
		{name: "missing quota catalog", status: http.StatusOK, body: `{"base_resp":{"status_code":0}}`},
		{name: "provider failure code", status: http.StatusOK, body: `{"base_resp":{"status_code":1000}}`},
		{
			name:   "oversized response",
			status: http.StatusOK,
			body:   strings.Repeat("x", maxAPIKeyValidationResponseBytes+1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			validator := &apiKeyValidator{client: server.Client(), endpoint: server.URL}
			if err := validator.ValidateAPIKey(context.Background(), "sk-cp-key"); !errors.Is(err, ErrAPIKeyValidationUnavailable) {
				t.Fatalf("error = %v, want ErrAPIKeyValidationUnavailable", err)
			}
		})
	}
}

func TestAPIKeyValidatorRejectsTokenPlanCredentialFailures(t *testing.T) {
	for _, statusCode := range []int{1004, 1008, 2049} {
		t.Run(fmt.Sprint(statusCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"base_resp":{"status_code":%d}}`, statusCode)
			}))
			defer server.Close()

			validator := &apiKeyValidator{client: server.Client(), endpoint: server.URL}
			if err := validator.ValidateAPIKey(context.Background(), "sk-cp-key"); !errors.Is(err, agentauth.ErrAPIKeyRejected) {
				t.Fatalf("error = %v, want ErrAPIKeyRejected", err)
			}
		})
	}
}

func TestAPIKeyValidatorHonorsHTTPClientTimeout(t *testing.T) {
	client := &http.Client{
		Timeout: 20 * time.Millisecond,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}
	validator := &apiKeyValidator{client: client, endpoint: "https://api.minimax.invalid/v1/token_plan/remains"}
	if err := validator.ValidateAPIKey(context.Background(), "sk-cp-key"); !errors.Is(err, ErrAPIKeyValidationUnavailable) {
		t.Fatalf("error = %v, want ErrAPIKeyValidationUnavailable", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
