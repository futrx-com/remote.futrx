package kimi

import (
	"context"
	"encoding/json"
	"fmt"
)

const remoteBrowserName = "remote_browser"

func (r *serverRun) prepareBrowser(ctx context.Context, p *serverTransport) error {
	if !r.req.EnableBrowser {
		return nil
	}
	desired := map[string]any{"transport": "stdio", "command": "npx", "args": []string{"@playwright/mcp", "--cdp-endpoint", "http://127.0.0.1:9222", "--caps=vision"}}
	var existing struct {
		Config struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"config"`
	}
	err := p.api(ctx, "GET", "/api/v2/mcp/servers/"+remoteBrowserName, nil, &existing)
	if err == nil {
		args, _ := json.Marshal(existing.Config.Args)
		want, _ := json.Marshal(desired["args"])
		if existing.Config.Command != "npx" || string(args) != string(want) {
			return fmt.Errorf("Kimi MCP name %q is already configured differently", remoteBrowserName)
		}
		return nil
	}
	apiErr, ok := err.(*serverError)
	if !ok || apiErr.Code != 40408 {
		return err
	}
	// The dedicated entry is merged by Kimi's management API, which preserves
	// unrelated user/project/plugin entries and refuses read-only collisions.
	desired["name"] = remoteBrowserName
	return p.api(ctx, "POST", "/api/v2/mcp/servers", desired, nil)
}
