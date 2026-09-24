package httphandlers

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// The backend prefix has to be recognised exactly: too loose and an instance
// named "backendish" routes to a backend, too strict and nested routes break.
func TestIsBackendPath(t *testing.T) {
	for _, tc := range []struct {
		action string
		path   string
		want   bool
	}{
		{"backend", "", true},
		{"backend/", "", true},
		{"backend/health", "health", true},
		{"backend/kv/greeting", "kv/greeting", true},
		{"backendish", "", false},
		{"credentials", "", false},
		{"start", "", false},
		{"", "", false},
	} {
		path, ok := isBackendPath(tc.action)
		if ok != tc.want || path != tc.path {
			t.Errorf("isBackendPath(%q) = %q, %v; want %q, %v", tc.action, path, ok, tc.path, tc.want)
		}
	}
}

func TestReadBackendRequestBodyAcceptsTheExactLimit(t *testing.T) {
	want := bytes.Repeat([]byte("x"), maxBackendRequestBody)
	request := httptest.NewRequest(http.MethodPost, "/backend/upload", bytes.NewReader(want))
	response := httptest.NewRecorder()

	body, ok := readBackendRequestBody(response, request)
	if !ok {
		t.Fatalf("exact-limit body was rejected with status %d: %s", response.Code, response.Body.String())
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("body length = %d, want %d", len(body), len(want))
	}
}

func TestReadBackendRequestBodyRejectsRatherThanTruncatesOversizeInput(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/backend/upload",
		bytes.NewReader(bytes.Repeat([]byte("x"), maxBackendRequestBody+1)),
	)
	response := httptest.NewRecorder()

	body, ok := readBackendRequestBody(response, request)
	if ok || body != nil {
		t.Fatalf("oversized body was forwarded with %d bytes", len(body))
	}
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(response.Body.String(), "exceeds 1 MiB") {
		t.Fatalf("error body = %q", response.Body.String())
	}
}

type handlerReadSeekCloser struct {
	*bytes.Reader
	closed bool
}

func (r *handlerReadSeekCloser) Close() error {
	r.closed = true
	return nil
}

func streamResponse(body string, modTime time.Time) (applications.Response, *handlerReadSeekCloser) {
	content := &handlerReadSeekCloser{Reader: bytes.NewReader([]byte(body))}
	return applications.Stream(content, int64(len(body)), modTime, map[string][]string{
		"Content-Type":        {"text/plain; charset=utf-8"},
		"Content-Disposition": {`attachment; filename="example.txt"`},
		"Set-Cookie":          {"session=hijacked"},
		"Content-Length":      {"999"},
		"Content-Range":       {"bytes 99-100/101"},
		"Accept-Ranges":       {"none"},
		"Last-Modified":       {time.Unix(0, 0).UTC().Format(http.TimeFormat)},
	}), content
}

func TestWriteBackendResponseServesStreamRangesHeadAndConditionals(t *testing.T) {
	modTime := time.Unix(1_700_000_000, 0).UTC()
	tests := []struct {
		name       string
		method     string
		headers    map[string]string
		wantStatus int
		wantBody   string
		wantRange  string
		wantLength string
	}{
		{name: "full", method: http.MethodGet, wantStatus: http.StatusOK, wantBody: "0123456789", wantLength: "10"},
		{name: "range", method: http.MethodGet, headers: map[string]string{"Range": "bytes=2-5"}, wantStatus: http.StatusPartialContent, wantBody: "2345", wantRange: "bytes 2-5/10", wantLength: "4"},
		{name: "if-range matches", method: http.MethodGet, headers: map[string]string{"Range": "bytes=2-5", "If-Range": modTime.Format(http.TimeFormat)}, wantStatus: http.StatusPartialContent, wantBody: "2345", wantRange: "bytes 2-5/10", wantLength: "4"},
		{name: "if-range stale", method: http.MethodGet, headers: map[string]string{"Range": "bytes=2-5", "If-Range": modTime.Add(-time.Hour).Format(http.TimeFormat)}, wantStatus: http.StatusOK, wantBody: "0123456789", wantLength: "10"},
		{name: "head", method: http.MethodHead, wantStatus: http.StatusOK, wantBody: "", wantLength: "10"},
		{name: "not modified", method: http.MethodGet, headers: map[string]string{"If-Modified-Since": modTime.Format(http.TimeFormat)}, wantStatus: http.StatusNotModified, wantBody: ""},
		{name: "unsatisfiable", method: http.MethodGet, headers: map[string]string{"Range": "bytes=20-30"}, wantStatus: http.StatusRequestedRangeNotSatisfiable, wantBody: "invalid range: failed to overlap\n", wantRange: "bytes */10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response, content := streamResponse("0123456789", modTime)
			request := httptest.NewRequest(tc.method, "/backend/download", nil)
			for name, value := range tc.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			writeBackendResponse(recorder, request, response)

			result := recorder.Result()
			defer result.Body.Close()
			body, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if result.StatusCode != tc.wantStatus || string(body) != tc.wantBody {
				t.Fatalf("response = %d %q, want %d %q", result.StatusCode, body, tc.wantStatus, tc.wantBody)
			}
			if got := result.Header.Get("Content-Range"); got != tc.wantRange {
				t.Errorf("Content-Range = %q, want %q", got, tc.wantRange)
			}
			if got := result.Header.Get("Content-Length"); got != tc.wantLength {
				t.Errorf("Content-Length = %q, want %q", got, tc.wantLength)
			}
			if tc.wantStatus != http.StatusNotModified && tc.wantStatus != http.StatusRequestedRangeNotSatisfiable && result.Header.Get("Accept-Ranges") != "bytes" {
				t.Errorf("Accept-Ranges = %q", result.Header.Get("Accept-Ranges"))
			}
			if result.Header.Get("Set-Cookie") != "" {
				t.Error("stream response kept Set-Cookie")
			}
			if result.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Error("stream response is sniffable")
			}
			if tc.wantStatus != http.StatusRequestedRangeNotSatisfiable {
				if got := result.Header.Get("Last-Modified"); got != modTime.Format(http.TimeFormat) {
					t.Errorf("Last-Modified = %q, want declared mod time", got)
				}
			}
			if !content.closed {
				t.Error("stream content was not closed")
			}
		})
	}
}

type handlerBlockingStream struct {
	size      int64
	position  int64
	started   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func (s *handlerBlockingStream) Read([]byte) (int, error) {
	s.startOnce.Do(func() { close(s.started) })
	<-s.closed
	return 0, net.ErrClosed
}

func (s *handlerBlockingStream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		s.position = offset
	case io.SeekCurrent:
		s.position += offset
	case io.SeekEnd:
		s.position = s.size + offset
	}
	return s.position, nil
}

func (s *handlerBlockingStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func TestWriteBackendResponseCancellationClosesStream(t *testing.T) {
	content := &handlerBlockingStream{
		size:    1,
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	response := applications.Stream(content, 1, time.Time{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/backend/download", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		writeBackendResponse(httptest.NewRecorder(), request, response)
		close(done)
	}()

	select {
	case <-content.started:
	case <-time.After(time.Second):
		t.Fatal("stream read did not start")
	}
	cancel()
	select {
	case <-content.closed:
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not close stream")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler remained blocked after cancellation")
	}
}

// A backend is told who the caller is; it is not given the means to become
// them. Forwarding the session cookie would hand every backend the ability to
// act as the signed-in user against the rest of the API.
func TestForwardableHeadersWithholdsCredentials(t *testing.T) {
	forwarded := forwardableHeaders(http.Header{
		"Cookie":            {"session=secret"},
		"Authorization":     {"Bearer token"},
		"Connection":        {"keep-alive"},
		"Transfer-Encoding": {"chunked"},
		"Content-Type":      {"application/json"},
		"X-Request-Id":      {"abc"},
	})
	for _, withheld := range []string{"Cookie", "Authorization", "Connection", "Transfer-Encoding"} {
		if _, present := forwarded[withheld]; present {
			t.Errorf("%s was forwarded to the backend", withheld)
		}
	}
	if got := forwarded["Content-Type"]; len(got) != 1 || got[0] != "application/json" {
		t.Errorf("Content-Type = %v, want it forwarded", got)
	}
	if got := forwarded["X-Request-Id"]; len(got) != 1 {
		t.Errorf("X-Request-Id = %v, want it forwarded", got)
	}
}

// A backend's response is same-origin with the SPA, so it must not be able to
// write the session or have its body reinterpreted by the browser.
func TestWriteBackendResponseSanitizesHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/backend/test", nil)
	writeBackendResponse(recorder, request, applications.Response{
		Status: http.StatusCreated,
		Headers: map[string][]string{
			"Content-Type":      {"application/json; charset=utf-8"},
			"Set-Cookie":        {"session=hijacked"},
			"Connection":        {"close"},
			"Content-Length":    {"999"},
			"Cache-Control":     {"no-store"},
			"X-Backend-Verdict": {"ok"},
		},
		Body: []byte(`{"ok":true}`),
	})
	result := recorder.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusCreated {
		t.Errorf("status = %d", result.StatusCode)
	}
	for _, dropped := range []string{"Set-Cookie", "Connection", "Content-Length"} {
		if value := result.Header.Get(dropped); value != "" {
			t.Errorf("%s survived as %q", dropped, value)
		}
	}
	if result.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the response is sniffable")
	}
	for header, want := range map[string]string{
		"Content-Type":      "application/json; charset=utf-8",
		"Cache-Control":     "no-store",
		"X-Backend-Verdict": "ok",
	} {
		if got := result.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

// A backend that answers with nothing still has to produce a valid response,
// and one that cannot be executed by the browser.
func TestWriteBackendResponseDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   int
	}{
		{"unset status", 0, http.StatusOK},
		{"nonsense status", 42, http.StatusOK},
		{"backend error", http.StatusBadGateway, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/backend/test", nil)
			writeBackendResponse(recorder, request, applications.Response{Status: tc.status})
			if recorder.Code != tc.want {
				t.Errorf("status = %d, want %d", recorder.Code, tc.want)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("Content-Type = %q, want a non-executable default", got)
			}
		})
	}
}
