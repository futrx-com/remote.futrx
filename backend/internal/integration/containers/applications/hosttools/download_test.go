package hosttools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadStallStopsHeadersOrBodyWithoutRetry(t *testing.T) {
	for _, stage := range []string{"headers", "body"} {
		t.Run(stage, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if stage == "body" {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			started := time.Now()
			body, err := fetchDownload(context.Background(), server.Client(), server.URL, 30*time.Millisecond)
			if err == nil {
				_, err = io.ReadAll(body)
				body.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "connection stalled") {
				t.Fatalf("stall error=%v", err)
			}
			if requests.Load() != 1 || time.Since(started) > time.Second {
				t.Fatal("stalled request retried or exceeded its deadline")
			}
		})
	}
}

func TestDownloadMayOutliveIdleDeadlineWhileMakingProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i < 12; i++ {
			_, _ = io.WriteString(w, "chunk\n")
			w.(http.Flusher).Flush()
			time.Sleep(15 * time.Millisecond)
		}
	}))
	defer server.Close()
	body, err := fetchDownload(context.Background(), server.Client(), server.URL, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || strings.Count(string(data), "chunk") != 12 {
		t.Fatalf("progressing download=%q %v", data, err)
	}
}
