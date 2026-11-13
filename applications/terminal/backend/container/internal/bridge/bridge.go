//go:build linux

// Package bridge carries Terminal's host backend requests to the service
// socket. The host keeps one `lxc exec … remote-terminal bridge` process per
// installation and writes one JSON request per line on its stdin; each is
// answered on stdout, tagged with its ID, as soon as the service replies. The
// requests run concurrently, so an output read that is waiting for the shell
// does not hold up typing.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"futrx.local/catalog/applications/terminal/backend/container/internal/config"
)

// Request is one host call. Body is base64 in JSON.
type Request struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query,omitempty"`
	Body   []byte `json:"body,omitempty"`
}

// Response answers the request with the same ID.
type Response struct {
	ID     uint64 `json:"id"`
	Status int    `json:"status"`
	Body   []byte `json:"body,omitempty"`
}

var routes = map[string]bool{"/sessions": true, "/output": true, "/input": true, "/resize": true}

// Run relays requests from in to the service at socket until in ends, then
// abandons whatever is still waiting.
func Run(in io.Reader, out io.Writer, socket string) error {
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
		// Every waiting output read holds one connection.
		MaxIdleConnsPerHost: config.BridgeIdleConnections,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var writeMu sync.Mutex
	encoder := json.NewEncoder(out)
	reply := func(response Response) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = encoder.Encode(response)
	}

	var running sync.WaitGroup
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64<<10), config.MaxBridgeFrameBytes)
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		running.Add(1)
		go func() {
			defer running.Done()
			reply(forward(ctx, client, request))
		}()
	}
	cancel()
	running.Wait()
	return scanner.Err()
}

func forward(ctx context.Context, client *http.Client, request Request) Response {
	if !routes[request.Path] || (request.Method != http.MethodGet && request.Method != http.MethodPost) ||
		strings.ContainsAny(request.Query, "#") {
		return failure(request.ID, http.StatusNotFound, "Unknown terminal route.")
	}
	target := "http://remote-terminal" + request.Path
	if request.Query != "" {
		target += "?" + request.Query
	}
	call, err := http.NewRequestWithContext(ctx, request.Method, target, bytes.NewReader(request.Body))
	if err != nil {
		return failure(request.ID, http.StatusBadRequest, "Invalid terminal request.")
	}
	response, err := client.Do(call)
	if err != nil {
		return failure(request.ID, http.StatusBadGateway, "The terminal service in the project container is not running.")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, config.MaxBridgeResponseBytes+1))
	if err != nil || len(body) > config.MaxBridgeResponseBytes {
		return failure(request.ID, http.StatusBadGateway, "Unreadable answer from the terminal service.")
	}
	return Response{ID: request.ID, Status: response.StatusCode, Body: body}
}

func failure(id uint64, status int, message string) Response {
	body, _ := json.Marshal(map[string]string{"error": message})
	return Response{ID: id, Status: status, Body: body}
}
