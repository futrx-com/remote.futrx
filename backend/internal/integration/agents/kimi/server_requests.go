package kimi

// These DTOs belong to Kimi's private REST/stdio boundary. Optional pointers
// distinguish omitted overrides from explicit false, empty string, or [].
type bridgeRequest struct {
	Type    string              `json:"type"`
	ID      int                 `json:"id"`
	Method  string              `json:"method,omitempty"`
	Path    string              `json:"path,omitempty"`
	Body    any                 `json:"body,omitempty"`
	Payload *nativeSubscription `json:"payload,omitempty"`
}

type nativeSubscription struct {
	Sessions []string                `json:"session_ids"`
	Cursors  map[string]nativeCursor `json:"cursors"`
}

type nativeCursor struct {
	Seq   int64  `json:"seq"`
	Epoch string `json:"epoch"`
}

type nativeAgentConfig struct {
	PlanMode       *bool  `json:"plan_mode,omitempty"`
	Model          string `json:"model,omitempty"`
	Thinking       string `json:"thinking,omitempty"`
	PermissionMode string `json:"permission_mode,omitempty"`
	SwarmMode      *bool  `json:"swarm_mode,omitempty"`
	TowerMode      *bool  `json:"tower_mode,omitempty"`
	GoalControl    string `json:"goal_control,omitempty"`
	GoalObjective  string `json:"goal_objective,omitempty"`
}

type nativeProfileUpdate struct {
	AgentConfig *nativeAgentConfig  `json:"agent_config,omitempty"`
	Metadata    *nativeCronMetadata `json:"metadata,omitempty"`
}

type nativeCronMetadata struct {
	Jobs map[string]bool `json:"remote_kimi_cron_jobs"`
}

type nativePrompt struct {
	Content       []nativeTextPart        `json:"content"`
	DisabledTools *[]string               `json:"disabled_tools,omitempty"`
	AgentID       *string                 `json:"agent_id,omitempty"`
	Profile       string                  `json:"profile,omitempty"`
	Model         *string                 `json:"model,omitempty"`
	Thinking      string                  `json:"thinking,omitempty"`
	Skills        []nativeSkillActivation `json:"skills,omitempty"`
}

type nativeTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type nativeSkillActivation struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

func (p *nativePrompt) setText(text string) {
	p.Content = []nativeTextPart{{Type: "text", Text: text}}
}
