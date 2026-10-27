package versiontelemetry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	serviceversiontelemetry "github.com/futrx-com/remote.futrx.com/internal/service/versiontelemetry"
)

func TestReportVersionPostsMinimalPayload(t *testing.T) {
	type requestRecord struct {
		method      string
		path        string
		contentType string
		userAgent   bool
		body        string
	}
	records := make(chan requestRecord, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasUserAgent := r.Header["User-Agent"]
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		records <- requestRecord{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			userAgent:   hasUserAgent,
			body:        string(body),
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newClient(server.URL+"/api/telemetry/version", server.Client())
	report := serviceversiontelemetry.Report{
		InstallationID: "abababababababababababababababab",
		Version:        "0.21.0",
	}
	if err := client.ReportVersion(context.Background(), report); err != nil {
		t.Fatal(err)
	}

	record := <-records
	if record.method != http.MethodPost || record.path != "/api/telemetry/version" {
		t.Fatalf("request = %s %s", record.method, record.path)
	}
	if record.contentType != "application/json" {
		t.Fatalf("content type = %q", record.contentType)
	}
	if record.userAgent {
		t.Fatal("request unexpectedly included User-Agent")
	}
	wantBody := `{"installationId":"abababababababababababababababab","version":"0.21.0"}`
	if record.body != wantBody {
		t.Fatalf("body = %q, want %q", record.body, wantBody)
	}
}

func TestReportVersionRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client())
	err := client.ReportVersion(context.Background(), serviceversiontelemetry.Report{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("error = %v, want HTTP 503", err)
	}
}

func TestReportVersionRejectsRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client := newClient(server.URL, newHTTPClient(time.Second))
	err := client.ReportVersion(context.Background(), serviceversiontelemetry.Report{})
	if err == nil || !errors.Is(err, errRedirectRejected) {
		t.Fatalf("error = %v, want redirect rejection", err)
	}
	if redirected.Load() != 0 {
		t.Fatal("telemetry client followed redirect")
	}
}

func TestReportVersionHonorsShortHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newClient(server.URL, newHTTPClient(20*time.Millisecond))
	started := time.Now()
	err := client.ReportVersion(context.Background(), serviceversiontelemetry.Report{})
	if err == nil {
		t.Fatal("timeout returned no error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s, want under 1s", elapsed)
	}
}
