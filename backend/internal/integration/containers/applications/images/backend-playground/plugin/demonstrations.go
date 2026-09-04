package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// slow is how to see the image's timeoutMs from the outside: ask for longer
// than it and the call fails while the plugin keeps running, so the next
// request succeeds.
func (p *playground) slow(request appplugin.Request) appplugin.Response {
	milliseconds := 250
	if raw := request.QueryValue("ms"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &milliseconds); err != nil {
			return appplugin.Errorf(http.StatusBadRequest, "ms must be a number")
		}
	}
	if milliseconds < 0 || milliseconds > 60000 {
		return appplugin.Errorf(http.StatusBadRequest, "ms must be between 0 and 60000")
	}
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	return appplugin.JSON(http.StatusOK, map[string]any{"sleptMs": milliseconds})
}

// boom asserts the robustness claim: a panicking route costs its caller one
// failed request. The process survives, and health still answers with the same
// pid afterwards.
func (p *playground) boom(appplugin.Request) appplugin.Response {
	panic("backend-playground: deliberate panic")
}

// adminOnly is a plugin authorizing its own callers. The image's access level
// decides who may reach the plugin at all; everything finer than that is the
// plugin's own job, and this is what it looks like.
func (p *playground) adminOnly(request appplugin.Request) appplugin.Response {
	if !request.Caller.IsAdmin {
		return appplugin.Errorf(http.StatusForbidden,
			"%s is not an administrator", request.Caller.Email)
	}
	return appplugin.JSON(http.StatusOK, map[string]any{
		"secret": "only administrators see this",
		"caller": request.Caller.Email,
	})
}
