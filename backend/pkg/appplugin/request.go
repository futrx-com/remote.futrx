package appplugin

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Tail returns the part of Path after prefix, or "" when Path does not start
// with it. It is the companion to a "kv/*" route pattern.
func (r Request) Tail(prefix string) string {
	if !strings.HasPrefix(r.Path, prefix) {
		return ""
	}
	return strings.TrimPrefix(r.Path, prefix)
}

// QueryValue returns the first value for a query parameter, or "".
func (r Request) QueryValue(key string) string {
	values := r.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Header returns the first value for a header, matched case-insensitively.
func (r Request) Header(name string) string {
	for key, values := range r.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// DecodeJSON unmarshals the request body into target.
func (r Request) DecodeJSON(target any) error {
	if len(r.Body) == 0 {
		return fmt.Errorf("appplugin: empty request body")
	}
	return json.Unmarshal(r.Body, target)
}
