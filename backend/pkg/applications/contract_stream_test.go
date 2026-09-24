package applications

import (
	"bytes"
	"io"
	"testing"
	"time"
)

type testReadSeekCloser struct {
	*bytes.Reader
	closed bool
}

func (r *testReadSeekCloser) Close() error {
	r.closed = true
	return nil
}

func TestStreamCreatesAnOwnedSeekableResponse(t *testing.T) {
	content := &testReadSeekCloser{Reader: bytes.NewReader([]byte("seekable"))}
	modTime := time.Unix(1_700_000_000, 0).UTC()
	headers := map[string][]string{"Content-Type": {"text/plain"}}

	response := Stream(content, 8, modTime, headers)
	headers["Content-Type"][0] = "mutated"

	if response.Status != 200 {
		t.Fatalf("status = %d, want 200", response.Status)
	}
	if response.Body != nil {
		t.Fatalf("buffered body = %q, want nil", response.Body)
	}
	if got := response.Headers["Content-Type"][0]; got != "text/plain" {
		t.Fatalf("header = %q, want a defensive copy", got)
	}
	gotContent, size, gotModTime, ok := response.ResponseStream()
	if !ok || gotContent != content || size != 8 || !gotModTime.Equal(modTime) {
		t.Fatalf("stream = (%T, %d, %s, %v)", gotContent, size, gotModTime, ok)
	}
	data, err := io.ReadAll(gotContent)
	if err != nil || string(data) != "seekable" {
		t.Fatalf("read = %q, %v", data, err)
	}
	if err := gotContent.Close(); err != nil || !content.closed {
		t.Fatalf("close = %v, closed = %v", err, content.closed)
	}
}

func TestBufferedResponseHasNoStream(t *testing.T) {
	if _, _, _, ok := Text(200, "buffered").ResponseStream(); ok {
		t.Fatal("buffered response reported a stream")
	}
}
