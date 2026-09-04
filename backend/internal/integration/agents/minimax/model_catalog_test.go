package minimax

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

func TestModelCatalogDiscoversLanguageModelsFromMiniMax(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-cp-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[
				{"id":"MiniMax-M3","object":"model"},
				{"id":"MiniMax-M2.7","object":"model"},
				{"id":"MiniMax-M2.7","object":"model"},
				{"id":"MiniMax-Speech-02-HD","object":"model"},
				{"id":"image-01","object":"model"},
				{"id":"MiniMax-M4/future","object":"model"},
				{"id":"MiniMax-M4 unsafe","object":"model"}
			]
		}`))
	}))
	defer server.Close()

	catalog := &modelCatalogClient{client: server.Client(), endpoint: server.URL + "/v1/models"}
	models, err := catalog.Models(context.Background(), "sk-cp-key")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M4/future"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestModelCatalogRejectsCredentialWithoutLeakingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"body-must-not-escape"}`))
	}))
	defer server.Close()

	catalog := &modelCatalogClient{client: server.Client(), endpoint: server.URL}
	_, err := catalog.Models(context.Background(), "sk-cp-revoked")
	if !errors.Is(err, ErrMiniMaxModelDiscoveryUnavailable) || !errors.Is(err, agentauth.ErrAPIKeyRejected) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "body-must-not-escape") {
		t.Fatalf("provider body escaped into error: %v", err)
	}
}

func TestModelCatalogTreatsProviderAndPayloadFailuresAsTemporary(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "provider error", status: http.StatusInternalServerError, body: `{}`},
		{name: "malformed JSON", status: http.StatusOK, body: `{`},
		{name: "empty catalog", status: http.StatusOK, body: `{"data":[]}`},
		{name: "media only", status: http.StatusOK, body: `{"data":[{"id":"image-01"}]}`},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("x", maxModelCatalogResponseBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			catalog := &modelCatalogClient{client: server.Client(), endpoint: server.URL}
			if _, err := catalog.Models(context.Background(), "sk-cp-key"); !errors.Is(err, ErrMiniMaxModelDiscoveryUnavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestModelCatalogHonorsHTTPClientTimeout(t *testing.T) {
	client := &http.Client{
		Timeout: 20 * time.Millisecond,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}
	catalog := &modelCatalogClient{client: client, endpoint: "https://api.minimax.invalid/v1/models"}
	if _, err := catalog.Models(context.Background(), "sk-cp-key"); !errors.Is(err, ErrMiniMaxModelDiscoveryUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveMiniMaxModelUsesProviderDefaultAndRejectsUnavailableSelection(t *testing.T) {
	models := []string{"MiniMax-M3", "MiniMax-M2.7"}
	if got, err := resolveMiniMaxModel(models, ""); err != nil || got != "MiniMax-M3" {
		t.Fatalf("auto = (%q, %v)", got, err)
	}
	if got, err := resolveMiniMaxModel(models, "MiniMax-M2.7"); err != nil || got != "MiniMax-M2.7" {
		t.Fatalf("selection = (%q, %v)", got, err)
	}
	for _, requested := range []string{"MiniMax-M2.6", `MiniMax-M3\"`} {
		if _, err := resolveMiniMaxModel(models, requested); !errors.Is(err, ErrMiniMaxModelUnavailable) {
			t.Fatalf("resolve %q error = %v", requested, err)
		}
	}
}
