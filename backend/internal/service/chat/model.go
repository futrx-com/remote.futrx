package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type ID string
type ProjectID string
type Provider = agent.ProviderID
type SessionIDs map[Provider]string

const (
	ProviderClaude      = agent.ProviderClaude
	ProviderCodex       = agent.ProviderCodex
	ProviderKimi        = agent.ProviderKimi
	ProviderAntigravity = agent.ProviderAntigravity
)

type Meta struct {
	ID                   ID         `json:"id"`
	Title                string     `json:"title"`
	Provider             Provider   `json:"provider,omitempty"`
	Sessions             SessionIDs `json:"sessions,omitempty"`
	ClaudeSessionID      string     `json:"claudeSessionId,omitempty"`
	CodexSessionID       string     `json:"codexSessionId,omitempty"`
	KimiSessionID        string     `json:"kimiSessionId,omitempty"`
	AntigravitySessionID string     `json:"antigravitySessionId,omitempty"`
	TmuxSession          string     `json:"tmuxSession,omitempty"`
	Cwd                  string     `json:"cwd,omitempty"`
	CreatedAt            int64      `json:"createdAt"`
	LastMessageAt        int64      `json:"lastMessageAt"`
	LastReadAt           int64      `json:"lastReadAt,omitempty"`
	Running              bool       `json:"running,omitempty"`
	Model                string     `json:"model"`
	Mode                 string     `json:"mode"`
	ReasoningEffort      string     `json:"reasoningEffort"`
	ServiceTier          string     `json:"serviceTier"`
	ApprovalPolicy       string     `json:"approvalPolicy"`
	SandboxPolicy        string     `json:"sandboxPolicy"`
	ProjectID            ProjectID  `json:"projectId,omitempty"`
	ForkPending          bool       `json:"forkPending,omitempty"`
	SelectedSkills       []SkillRef `json:"selectedSkills,omitempty"`
}

type SkillRef struct {
	Name     string   `json:"name"`
	Command  string   `json:"command,omitempty"`
	Provider Provider `json:"provider,omitempty"`
	Source   string   `json:"source,omitempty"`
}

type Event struct {
	Seq                  int64           `json:"seq,omitempty"`
	T                    int64           `json:"t"`
	Type                 string          `json:"type"`
	TurnID               string          `json:"turnId,omitempty"`
	Text                 string          `json:"text,omitempty"`
	MessageID            string          `json:"messageId,omitempty"`
	ID                   string          `json:"id,omitempty"`
	Name                 string          `json:"name,omitempty"`
	Input                json.RawMessage `json:"input,omitempty"`
	Output               string          `json:"output,omitempty"`
	IsError              bool            `json:"isError,omitempty"`
	ToolName             string          `json:"toolName,omitempty"`
	Subtype              string          `json:"subtype,omitempty"`
	Data                 json.RawMessage `json:"data,omitempty"`
	SessionID            string          `json:"sessionId,omitempty"`
	ClaudeSessionID      string          `json:"claudeSessionId,omitempty"`
	CodexSessionID       string          `json:"codexSessionId,omitempty"`
	KimiSessionID        string          `json:"kimiSessionId,omitempty"`
	AntigravitySessionID string          `json:"antigravitySessionId,omitempty"`
	Provider             Provider        `json:"provider,omitempty"`
	Usage                json.RawMessage `json:"usage,omitempty"`
	Message              string          `json:"message,omitempty"`
	Running              bool            `json:"running,omitempty"`
	// ScheduledTaskID marks events produced by a scheduled run rather than an
	// interactive one, so consumers can tell "your turn finished" from "a task
	// ran while you were away".
	ScheduledTaskID string                `json:"scheduledTaskId,omitempty"`
	Native          *agent.NativeEnvelope `json:"native,omitempty"`
	InteractionID   string                `json:"interactionId,omitempty"`
	Status          string                `json:"status,omitempty"`
}

// NormalizeSessions makes the provider-keyed session map authoritative while
// preserving the four legacy JSON fields during the compatibility window.
// Legacy records are imported when no generic value exists.
func (m *Meta) NormalizeSessions() {
	if m == nil {
		return
	}
	sessions := cloneSessions(m.Sessions)
	for provider, legacy := range map[Provider]string{
		ProviderClaude:      m.ClaudeSessionID,
		ProviderCodex:       m.CodexSessionID,
		ProviderKimi:        m.KimiSessionID,
		ProviderAntigravity: m.AntigravitySessionID,
	} {
		if _, exists := sessions[provider]; !exists && strings.TrimSpace(legacy) != "" {
			if sessions == nil {
				sessions = make(SessionIDs)
			}
			sessions[provider] = legacy
		}
	}
	for provider, sessionID := range sessions {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			delete(sessions, provider)
			continue
		}
		sessions[provider] = sessionID
	}
	if len(sessions) == 0 {
		sessions = nil
	}
	m.Sessions = sessions
	m.syncLegacySessions()
}

func (m Meta) SessionID(provider Provider) string {
	if sessionID := strings.TrimSpace(m.Sessions[provider]); sessionID != "" {
		return sessionID
	}
	switch provider {
	case ProviderClaude:
		return m.ClaudeSessionID
	case ProviderCodex:
		return m.CodexSessionID
	case ProviderKimi:
		return m.KimiSessionID
	case ProviderAntigravity:
		return m.AntigravitySessionID
	default:
		return ""
	}
}

func (m *Meta) SetSessionID(provider Provider, sessionID string) {
	if m == nil || provider == "" {
		return
	}
	m.NormalizeSessions()
	if m.Sessions == nil && strings.TrimSpace(sessionID) != "" {
		m.Sessions = make(SessionIDs)
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID == "" {
		delete(m.Sessions, provider)
	} else {
		m.Sessions[provider] = sessionID
	}
	if len(m.Sessions) == 0 {
		m.Sessions = nil
	}
	m.syncLegacySessions()
}

func (m *Meta) ClearSessionIDs() {
	if m == nil {
		return
	}
	m.Sessions = nil
	m.ClaudeSessionID = ""
	m.CodexSessionID = ""
	m.KimiSessionID = ""
	m.AntigravitySessionID = ""
}

func (m Meta) SessionSnapshot() SessionIDs {
	m.NormalizeSessions()
	return cloneSessions(m.Sessions)
}

func (m *Meta) syncLegacySessions() {
	m.ClaudeSessionID = m.Sessions[ProviderClaude]
	m.CodexSessionID = m.Sessions[ProviderCodex]
	m.KimiSessionID = m.Sessions[ProviderKimi]
	m.AntigravitySessionID = m.Sessions[ProviderAntigravity]
}

// SetSession records a generic session event and mirrors known provider IDs to
// the legacy event fields consumed by older frontend builds.
func (e *Event) SetSession(provider Provider, sessionID string) {
	if e == nil {
		return
	}
	e.Provider = provider
	e.SessionID = strings.TrimSpace(sessionID)
	e.ClaudeSessionID = ""
	e.CodexSessionID = ""
	e.KimiSessionID = ""
	e.AntigravitySessionID = ""
	switch provider {
	case ProviderClaude:
		e.ClaudeSessionID = e.SessionID
	case ProviderCodex:
		e.CodexSessionID = e.SessionID
	case ProviderKimi:
		e.KimiSessionID = e.SessionID
	case ProviderAntigravity:
		e.AntigravitySessionID = e.SessionID
	}
}

// NormalizeSession imports legacy event fields and then mirrors the generic
// provider/session pair back to those fields for old clients.
func (e *Event) NormalizeSession() {
	if e == nil {
		return
	}
	provider := e.Provider
	sessionID := strings.TrimSpace(e.SessionID)
	if sessionID == "" {
		legacy := []struct {
			provider  Provider
			sessionID string
		}{
			{ProviderClaude, e.ClaudeSessionID},
			{ProviderCodex, e.CodexSessionID},
			{ProviderKimi, e.KimiSessionID},
			{ProviderAntigravity, e.AntigravitySessionID},
		}
		for _, candidate := range legacy {
			if provider != "" && candidate.provider != provider {
				continue
			}
			if strings.TrimSpace(candidate.sessionID) != "" {
				provider = candidate.provider
				sessionID = candidate.sessionID
				break
			}
		}
	}
	e.SetSession(provider, sessionID)
}

func cloneSessions(sessions SessionIDs) SessionIDs {
	if len(sessions) == 0 {
		return nil
	}
	cloned := make(SessionIDs, len(sessions))
	for provider, sessionID := range sessions {
		cloned[provider] = sessionID
	}
	return cloned
}

type EventPageQuery struct {
	Limit     int
	BeforeSeq int64
}

type EventPage struct {
	Events     []Event `json:"events"`
	NextBefore int64   `json:"nextBefore,omitempty"`
	LastSeq    int64   `json:"lastSeq"`
	HasMore    bool    `json:"hasMore"`
}

type CreateInput struct {
	Title           string     `json:"title,omitempty"`
	TmuxSession     string     `json:"tmuxSession,omitempty"`
	Cwd             string     `json:"cwd,omitempty"`
	Provider        Provider   `json:"provider,omitempty"`
	Model           string     `json:"model,omitempty"`
	Mode            string     `json:"mode,omitempty"`
	ReasoningEffort string     `json:"reasoningEffort,omitempty"`
	ServiceTier     string     `json:"serviceTier,omitempty"`
	ApprovalPolicy  string     `json:"approvalPolicy,omitempty"`
	SandboxPolicy   string     `json:"sandboxPolicy,omitempty"`
	ProjectID       ProjectID  `json:"projectId,omitempty"`
	SelectedSkills  []SkillRef `json:"selectedSkills,omitempty"`
}

type UpdateInput struct {
	Title           *string     `json:"title,omitempty"`
	Cwd             *string     `json:"cwd,omitempty"`
	Provider        *Provider   `json:"provider,omitempty"`
	Model           *string     `json:"model,omitempty"`
	Mode            *string     `json:"mode,omitempty"`
	ReasoningEffort *string     `json:"reasoningEffort,omitempty"`
	ServiceTier     *string     `json:"serviceTier,omitempty"`
	ApprovalPolicy  *string     `json:"approvalPolicy,omitempty"`
	SandboxPolicy   *string     `json:"sandboxPolicy,omitempty"`
	SelectedSkills  *[]SkillRef `json:"selectedSkills,omitempty"`
}

func NormalizeProvider(provider Provider) Provider {
	normalized := agent.NormalizeProviderID(string(provider))
	if normalized == "" || !agent.ValidProviderID(normalized) {
		return ProviderCodex
	}
	return normalized
}

func NormalizeReasoningEffort(effort string) string {
	return agent.NormalizePreferenceValue(effort)
}

// NormalizeServiceTier keeps future provider tiers usable without requiring a
// frontend/backend release for every catalog addition. "" means Auto.
func NormalizeServiceTier(tier string) string {
	return agent.NormalizePreferenceValue(tier)
}

func NormalizeApprovalPolicy(policy string) string {
	return agent.NormalizeApprovalPolicy(policy)
}

func NormalizeSandboxPolicy(policy string) string {
	return agent.NormalizeSandboxPolicy(policy)
}

func NormalizeSelectedSkills(skills []SkillRef, fallbackProvider Provider) []SkillRef {
	fallbackProvider = NormalizeProvider(fallbackProvider)
	seen := map[string]bool{}
	normalized := make([]SkillRef, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		command := strings.TrimSpace(skill.Command)
		source := strings.TrimSpace(skill.Source)
		if command == "" {
			command = name
		}
		if name == "" {
			name = command
		}
		if name == "" || command == "" {
			continue
		}

		provider := skill.Provider
		if provider == "" {
			provider = fallbackProvider
		} else {
			provider = NormalizeProvider(provider)
		}
		key := strings.ToLower(string(provider)) + "\x00" + strings.ToLower(source) + "\x00" + strings.ToLower(command)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, SkillRef{
			Name:     name,
			Command:  command,
			Provider: provider,
			Source:   source,
		})
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func ValidID(id ID) bool {
	if len(id) < 4 || len(id) > 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// TitleFromPrompt produces a short summary used when a chat is created with
// no explicit title. First 60 chars of the first prompt, single line.
func TitleFromPrompt(prompt string) string {
	t := strings.TrimSpace(prompt)
	t = strings.ReplaceAll(t, "\n", " ")
	t = strings.ReplaceAll(t, "\r", " ")
	for strings.Contains(t, "  ") {
		t = strings.ReplaceAll(t, "  ", " ")
	}
	if len(t) > 60 {
		t = t[:60] + "..."
	}
	if t == "" {
		t = fmt.Sprintf("Chat %s", time.Now().Format("Jan 2 15:04"))
	}
	return t
}
