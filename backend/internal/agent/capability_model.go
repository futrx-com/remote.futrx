package agent

type CapabilitySource string

const (
	CapabilitySourceLive     CapabilitySource = "live"
	CapabilitySourceFallback CapabilitySource = "fallback"
)

// CapabilityOption is the provider-neutral shape used by reasoning-effort,
// service-tier (speed), and provider-native mode selectors.
type CapabilityOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
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
}

type CapabilityAuthentication struct {
	Mode                string `json:"mode"`
	Instructions        string `json:"instructions,omitempty"`
	SatisfiesAccessGate bool   `json:"satisfiesAccessGate"`
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
