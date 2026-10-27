package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeAcceptsOnlySuccessfulResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := probe(context.Background(), server.URL); err != nil {
		t.Fatalf("probe successful service: %v", err)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "not ready", http.StatusServiceUnavailable)
	}))
	defer failing.Close()
	if err := probe(context.Background(), failing.URL); err == nil {
		t.Fatal("probe accepted a failing service")
	}
}
