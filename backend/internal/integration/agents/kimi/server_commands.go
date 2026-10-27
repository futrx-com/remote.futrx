package kimi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Commands are explicit user actions. Plain prompts are always sent verbatim;
// command arguments are never interpolated into a shell or an arbitrary URL.
func (r *serverRun) submit(ctx context.Context, p *serverTransport) (bool, error) {
	prompt := r.req.Prompt
	commandSource := r.req.UserPrompt
	if commandSource == "" {
		commandSource = prompt
	}
	commandLine, rest, hasRest := strings.Cut(strings.TrimSpace(commandSource), "\n")
	command, args, _ := strings.Cut(commandLine, " ")
	args = strings.TrimSpace(args)
	prefix := ""
	if r.req.UserPrompt != "" && strings.HasSuffix(prompt, r.req.UserPrompt) {
		prefix = strings.TrimSuffix(prompt, r.req.UserPrompt)
	}

	disabledTools := []string{}
	body := nativePrompt{DisabledTools: &disabledTools}
	body.setText(prompt)
	if !r.req.EnableBrowser {
		disabledTools = []string{"mcp__remote_browser__*"}
	}
	profile := func(config nativeAgentConfig) error {
		return p.api(ctx, "POST", r.path()+"/profile", nativeProfileUpdate{AgentConfig: &config}, nil)
	}
	done := func(message string, err error) (bool, error) {
		if err == nil {
			r.publish(agent.Event{Type: agent.EventAssistantTextDelta, Text: message})
		}
		return true, err
	}
	switch command {
	case "/btw":
		question := strings.TrimSpace(args + "\n" + rest)
		if question == "" {
			return true, fmt.Errorf("use /btw <side question>")
		}
		var side struct {
			AgentID string `json:"agent_id"`
		}
		if err := p.api(ctx, "POST", r.path()+":btw", map[string]any{}, &side); err != nil {
			return false, err
		}
		if side.AgentID == "" {
			return false, fmt.Errorf("Kimi did not return a side-conversation agent")
		}
		r.publishChild(r.activity.startSideConversation(side.AgentID, question), nil)
		r.mainEnded = true
		body.AgentID = &side.AgentID
		body.setText(prefix + question)
		body.DisabledTools = nil // Keep the native side agent's tool policy.
		return false, p.api(ctx, "POST", r.path()+"/prompts", body, nil)
	case "/tower":
		if args != "on" && args != "off" {
			return true, fmt.Errorf("use /tower on or /tower off")
		}
		enabled := args == "on"
		if err := profile(nativeAgentConfig{TowerMode: &enabled}); err != nil {
			return true, err
		}
		if !hasRest {
			return done("Kimi tower mode "+args+".", nil)
		}
	case "/swarm":
		enabled := args != "off"
		if args != "" && args != "on" && args != "off" {
			return true, fmt.Errorf("use /swarm on or /swarm off, followed by an optional prompt on the next line")
		}
		if err := profile(nativeAgentConfig{SwarmMode: &enabled}); err != nil {
			return true, err
		}
		if !hasRest {
			return done("Kimi swarm mode "+map[bool]string{true: "enabled.", false: "disabled."}[enabled], nil)
		}
	case "/goal":
		config := nativeAgentConfig{}
		switch args {
		case "pause", "resume", "cancel":
			config.GoalControl = args
		default:
			if args == "" {
				return true, fmt.Errorf("use /goal <objective>, /goal pause, /goal resume, or /goal cancel")
			}
			config.GoalObjective = args
		}
		if err := profile(config); err != nil {
			return true, err
		}
		if args == "pause" || args == "cancel" {
			return done("Kimi goal "+args+" requested.", nil)
		}
		if !hasRest {
			rest = "Continue the current goal."
		}
	case "/agent":
		if args == "" || !hasRest || strings.TrimSpace(rest) == "" {
			return true, fmt.Errorf("use /agent <profile> followed by a prompt on the next line")
		}
		body.Profile = args
		body.Model = &r.req.Model
		if r.thinking != "" {
			body.Thinking = r.thinking
		}
	case "/compact":
		r.compacting = true
		err := p.api(ctx, "POST", r.path()+":compact", map[string]any{"instruction": strings.TrimSpace(prefix + args + "\n" + rest)}, nil)
		return false, err
	case "/undo":
		count := 1
		if args != "" {
			var err error
			count, err = strconv.Atoi(args)
			if err != nil || count < 1 {
				return true, fmt.Errorf("use /undo or /undo <positive number of turns>")
			}
		}
		return done(fmt.Sprintf("Rewound Kimi's context by %d turn(s).", count), p.api(ctx, "POST", r.path()+":undo", map[string]any{"count": count}, nil))
	case "/kimi":
		if args == "" || args == "help" {
			return done("Kimi controls:\n- /agent <profile> followed by a prompt on the next line\n- /btw <side question>\n- /swarm on or /swarm off\n- /tower on or /tower off\n- /goal <objective>, pause, resume, or cancel\n- /compact [instructions]\n- /undo [turn count]\n- /skill:<name> [arguments]\nModels, Thinking, Plan mode, and approvals are in the composer. Ask Kimi to manage tasks, MCP servers, plugins, hooks, or scheduled work using its native tools.", nil)
		}
		return true, fmt.Errorf("unknown Kimi command; use /kimi help")
	default:
		if strings.HasPrefix(command, "/skill:") {
			name := strings.TrimPrefix(command, "/skill:")
			if name == "" {
				return true, fmt.Errorf("missing Kimi skill name")
			}
			body.Skills = []nativeSkillActivation{{Name: name, Args: strings.TrimSpace(args + "\n" + rest)}}
			body.setText(prefix + "Use the selected skill.")
			return false, p.api(ctx, "POST", r.path()+"/prompts", body, nil)
		}
		return false, p.api(ctx, "POST", r.path()+"/prompts", body, nil)
	}
	body.setText(prefix + rest)
	return false, p.api(ctx, "POST", r.path()+"/prompts", body, nil)
}

func (r *serverRun) userCommand() string {
	source := r.req.UserPrompt
	if source == "" {
		source = r.req.Prompt
	}
	fields := strings.Fields(source)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
