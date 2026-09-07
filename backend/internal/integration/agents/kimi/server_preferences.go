package kimi

import (
	"context"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// applyPreferences translates composer choices into native session settings.
// Model binding must still precede prompt submission except for /agent, which
// binds its selected profile together with model/thinking in the prompt call.
func (r *serverRun) applyPreferences(ctx context.Context, p *serverTransport) error {
	var defaults struct {
		Model    string `json:"default_model"`
		Thinking struct {
			Effort  string `json:"effort"`
			Enabled *bool  `json:"enabled"`
		} `json:"thinking"`
	}
	if r.req.Model == "" || r.req.Preferences.ReasoningEffort == "" {
		if err := p.api(ctx, "GET", "/api/v1/config", nil, &defaults); err != nil {
			return err
		}
	}
	if r.req.Model == "" {
		r.req.Model = defaults.Model
	}
	thinking := string(r.req.Preferences.ReasoningEffort)
	if thinking == "" {
		thinking = defaults.Thinking.Effort
		if defaults.Thinking.Enabled != nil && !*defaults.Thinking.Enabled {
			thinking = "off"
		}
		if thinking == "" {
			var catalog struct {
				Items []struct {
					Model        string   `json:"model"`
					Capabilities []string `json:"capabilities"`
					Efforts      []string `json:"support_efforts"`
				} `json:"items"`
			}
			if err := p.api(ctx, "GET", "/api/v1/models", nil, &catalog); err != nil {
				return err
			}
			thinking = "off"
			for _, model := range catalog.Items {
				if model.Model != r.req.Model {
					continue
				}
				if len(model.Efforts) > 0 {
					thinking = "on"
				}
				for _, cap := range model.Capabilities {
					if cap == "thinking" || cap == "always_thinking" {
						thinking = "on"
					}
				}
			}
		}
	}

	planMode := r.req.Mode == agent.RunModePlan
	config := nativeAgentConfig{PlanMode: &planMode}
	if model := normalizeKimiModel(r.req.Model); model != "" {
		config.Model = model
		r.usage.setModel(model)
	}
	config.Thinking = thinking
	r.thinking = thinking
	if r.userCommand() == "/agent" {
		config.Model = ""
		config.Thinking = ""
	}
	switch r.req.Preferences.ApprovalPolicy {
	case "never":
		config.PermissionMode = "auto"
	case "untrusted":
		config.PermissionMode = "manual"
	case "on-request":
		config.PermissionMode = "yolo"
	case "":
	default:
		return fmt.Errorf("unsupported Kimi approval policy %q", r.req.Preferences.ApprovalPolicy)
	}
	if err := p.api(ctx, "POST", r.path()+"/profile", nativeProfileUpdate{AgentConfig: &config}, nil); err != nil {
		return err
	}
	return nil
}
