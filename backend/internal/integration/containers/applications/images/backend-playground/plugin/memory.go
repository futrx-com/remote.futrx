package main

import (
	"net/http"
	"sort"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// kv holds state in the process. Writing a key and reading it back from a
// later request is the proof that a plugin is a long-lived process and not a
// function invoked per call.
func (p *playground) listValues(appplugin.Request) appplugin.Response {
	p.mu.Lock()
	defer p.mu.Unlock()
	keys := make([]string, 0, len(p.values))
	for key := range p.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return appplugin.JSON(http.StatusOK, map[string]any{"keys": keys})
}

func (p *playground) readValue(request appplugin.Request) appplugin.Response {
	key := request.Tail("kv/")
	if key == "" {
		return appplugin.Errorf(http.StatusBadRequest, "missing key")
	}
	p.mu.Lock()
	value, ok := p.values[key]
	p.mu.Unlock()
	if !ok {
		return appplugin.Errorf(http.StatusNotFound, "no value for %q", key)
	}
	return appplugin.JSON(http.StatusOK, map[string]string{"key": key, "value": value})
}

func (p *playground) writeValue(request appplugin.Request) appplugin.Response {
	key := request.Tail("kv/")
	if key == "" {
		return appplugin.Errorf(http.StatusBadRequest, "missing key")
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := request.DecodeJSON(&body); err != nil {
		return appplugin.Errorf(http.StatusBadRequest, "invalid json: %v", err)
	}
	p.mu.Lock()
	p.values[key] = body.Value
	p.mu.Unlock()
	return appplugin.JSON(http.StatusOK, map[string]string{"key": key, "value": body.Value})
}
