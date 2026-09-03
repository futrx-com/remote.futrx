package agent

import "encoding/json"

type CapabilitySource string

const (
	CapabilitySourceLive     CapabilitySource = "live"
	CapabilitySourceFallback CapabilitySource = "fallback"
)

// CapabilityOption is the provider-neutral shape used by reasoning-effort,
// service-tier (speed), and provider-native mode selectors.
type CapabilityOption struct {
	Value           string          `json:"value"`
	Label           string          `json:"label"`
	Description     string          `json:"description,omitempty"`
	Model           string          `json:"model,omitempty"`
	ReasoningEffort string          `json:"reasoningEffort,omitempty"`
	Raw             json.RawMessage `json:"raw,omitempty"`
}

// ModelCapability keeps controls next to the model that supports them. Some
// providers only publish provider-wide controls; their adapter expands those
// controls onto each returned model before exposing the catalog.
type ModelCapability struct {
	ID                     string             `json:"id"`
	Label                  string             `json:"label"`
	Description            string             `json:"description,omitempty"`
	ProviderDefault        bool               `json:"providerDefault,omitempty"`
	ReasoningEfforts       []CapabilityOption `json:"reasoningEfforts"`
	DefaultReasoningEffort string             `json:"defaultReasoningEffort,omitempty"`
	ServiceTiers           []CapabilityOption `json:"serviceTiers"`
	DefaultServiceTier     string             `json:"defaultServiceTier,omitempty"`
	InputModalities        []string           `json:"inputModalities,omitempty"`
	SupportsPersonality    bool               `json:"supportsPersonality,omitempty"`
	MultiAgentVersion      string             `json:"multiAgentVersion,omitempty"`
	Hidden                 bool               `json:"hidden,omitempty"`
	ModelSpecialty         string             `json:"modelSpecialty,omitempty"`
	Upgrade                string             `json:"upgrade,omitempty"`
	UpgradeInfo            json.RawMessage    `json:"upgradeInfo,omitempty"`
	AvailabilityNux        json.RawMessage    `json:"availabilityNux,omitempty"`
	Raw                    json.RawMessage    `json:"raw,omitempty"`
}

type CapabilityAuthentication struct {
	Mode                string                          `json:"mode"`
	Instructions        string                          `json:"instructions,omitempty"`
	SatisfiesAccessGate bool                            `json:"satisfiesAccessGate"`
	APIKey              *CapabilityAPIKeyAuthentication `json:"apiKey,omitempty"`
}

type CapabilityAPIKeyAuthentication struct {
	CreateURL       string `json:"createUrl"`
	CreateLabel     string `json:"createLabel"`
	CredentialLabel string `json:"credentialLabel"`
}

type CapabilitySessionSupport struct {
	Resume bool `json:"resume"`
	Fork   bool `json:"fork"`
}

type CapabilityFeatures struct {
	Sessions       CapabilitySessionSupport `json:"sessions"`
	Skills         string                   `json:"skills"`
	BrowserTools   bool                     `json:"browserTools"`
	ScheduledTools bool                     `json:"scheduledTools"`
}

// Capabilities is the normalized catalog returned by every agent adapter.
// Warning is intentionally concise and must not contain raw provider output.
type Capabilities struct {
	Provider          ProviderID               `json:"provider"`
	Label             string                   `json:"label"`
	Default           bool                     `json:"default,omitempty"`
	ExecutionScopes   []string                 `json:"executionScopes,omitempty"`
	Authentication    CapabilityAuthentication `json:"authentication"`
	Features          CapabilityFeatures       `json:"features"`
	Version           string                   `json:"version,omitempty"`
	Source            CapabilitySource         `json:"source"`
	Warning           string                   `json:"warning,omitempty"`
	UnavailableReason string                   `json:"unavailableReason,omitempty"`
	Models            []ModelCapability        `json:"models"`
	Modes             []CapabilityOption       `json:"modes"`
	DefaultMode       RunMode                  `json:"defaultMode,omitempty"`
}

type CapabilityRequest struct {
	// ContainerName scopes discovery to the project computer and therefore to
	// its installed CLI, credentials, and account entitlements. Empty means the
	// provider should inspect the host CLI.
	ContainerName string
}

// Clone returns a copy whose provider-owned metadata can be mutated without
// changing the source catalog.
func (capabilities Capabilities) Clone() Capabilities {
	cloned := capabilities
	cloned.ExecutionScopes = append([]string(nil), capabilities.ExecutionScopes...)
	cloned.Modes = cloneCapabilityOptions(capabilities.Modes)
	cloned.Models = make([]ModelCapability, len(capabilities.Models))
	for index, model := range capabilities.Models {
		cloned.Models[index] = cloneModelCapability(model)
	}
	return cloned
}

func cloneModelCapability(model ModelCapability) ModelCapability {
	cloned := model
	cloned.Raw = append([]byte(nil), model.Raw...)
	cloned.UpgradeInfo = append([]byte(nil), model.UpgradeInfo...)
	cloned.AvailabilityNux = append([]byte(nil), model.AvailabilityNux...)
	cloned.InputModalities = append([]string(nil), model.InputModalities...)
	cloned.ReasoningEfforts = cloneCapabilityOptions(model.ReasoningEfforts)
	cloned.ServiceTiers = cloneCapabilityOptions(model.ServiceTiers)
	return cloned
}

func cloneCapabilityOptions(options []CapabilityOption) []CapabilityOption {
	cloned := append([]CapabilityOption(nil), options...)
	for index := range cloned {
		cloned[index].Raw = append([]byte(nil), options[index].Raw...)
	}
	return cloned
}
