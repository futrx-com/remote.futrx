package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func (b *api) hello(request applications.Request) applications.Response {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Env carries the install's resolved inputs — here the greeting the user
	// typed into the install dialog, declared as env[] in application.json.
	greeting := b.instance.Env["HELLO_GREETING"]
	// Caller is stamped by the server from the session, never sent by the
	// browser, so a backend may trust it. The cookies that authenticated it are
	// withheld: this backend can tell who is asking, and cannot act as them.
	who := request.Caller.Email
	if who == "" {
		who = "there"
	}

	return applications.JSON(http.StatusOK, map[string]any{
		"message": fmt.Sprintf("%s, %s.", greeting, who),
		"scope":   b.instance.Scope,
		"project": b.instance.ProjectID,
		"admin":   request.Caller.IsAdmin,
		"visits":  b.visits,
	})
}

func (b *api) echo(request applications.Request) applications.Response {
	var body any
	if len(request.Body) > 0 {
		if err := json.Unmarshal(request.Body, &body); err != nil {
			body = string(request.Body)
		}
	}
	return applications.JSON(http.StatusOK, map[string]any{
		"method":  request.Method,
		"query":   request.Query,
		"headers": request.Headers,
		"body":    body,
	})
}
