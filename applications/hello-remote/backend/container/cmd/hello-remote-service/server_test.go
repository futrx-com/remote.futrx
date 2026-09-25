package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func encoded(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func TestConfigurationAndHealthResponse(t *testing.T) {
	values := map[string]string{
		"HELLO_GREETING_B64": encoded("Welcome"),
	}
	config, err := configurationFromEnv(func(name string) string { return values[name] })
	if err != nil {
		t.Fatal(err)
	}
	config.ProvisionedVersion = "10"

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	serviceHandler(config).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Message != "Welcome from the container service." ||
		body.ProvisionedVersion != "10" {
		t.Fatalf("health response = %+v", body)
	}
}
