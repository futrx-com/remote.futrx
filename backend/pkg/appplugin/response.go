package appplugin

import (
	"encoding/json"
	"fmt"
)

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
