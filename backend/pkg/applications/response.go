package applications

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// responseStream is kept out of the JSON and net/rpc wire shape. The RPC
// adapter recognizes it and exposes content through a bounded random-access
// reader on a brokered connection.
type responseStream struct {
	content io.ReadSeekCloser
	size    int64
	modTime time.Time
}

// Stream returns a response backed by seekable content without first reading
// it into memory. Remote takes ownership of content and closes it after the
// HTTP transfer, when the caller disconnects, or if transport setup fails.
//
// size must be the exact non-negative byte length. modTime may be zero; a
// non-zero value enables Last-Modified and conditional/range handling. Headers
// are copied. HTTP status, Range, HEAD, and conditional response semantics are
// owned by Remote's HTTP server, so streamed responses always begin as 200 and
// must not be combined with Body.
func Stream(
	content io.ReadSeekCloser,
	size int64,
	modTime time.Time,
	headers map[string][]string,
) Response {
	return Response{
		Status:  200,
		Headers: cloneHeaders(headers),
		stream: &responseStream{
			content: content,
			size:    size,
			modTime: modTime,
		},
	}
}

// ResponseStream returns the seekable content owned by a streamed response.
// It is transport plumbing: application backends normally only need Stream.
func (r Response) ResponseStream() (content io.ReadSeekCloser, size int64, modTime time.Time, ok bool) {
	if r.stream == nil {
		return nil, 0, time.Time{}, false
	}
	return r.stream.content, r.stream.size, r.stream.modTime, true
}

func cloneHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return nil
	}
	cloned := make(map[string][]string, len(headers))
	for name, values := range headers {
		cloned[name] = append([]string(nil), values...)
	}
	return cloned
}

// JSON encodes value as a JSON response body.
func JSON(status int, value any) Response {
	body, err := json.Marshal(value)
	if err != nil {
		return Errorf(500, "encode response: %v", err)
	}
	return Response{
		Status:  status,
		Headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}},
		Body:    body,
	}
}

// Text returns a plain-text response.
func Text(status int, body string) Response {
	return Response{
		Status:  status,
		Headers: map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}},
		Body:    []byte(body),
	}
}

// Errorf returns a JSON {"error": "..."} response, the shape the SPA's request
// helper already knows how to surface.
func Errorf(status int, format string, args ...any) Response {
	return JSON(status, map[string]string{"error": fmt.Sprintf(format, args...)})
}
