package constants

import "time"

const (
	KimiCodexLabel                 = "Kimi (Codex)"
	KimiCodexAutoModelLabel        = "Kimi default"
	KimiCodexAPIBaseURL            = "https://api.kimi.ai/coding/v1"
	KimiCodexModelsURL             = KimiCodexAPIBaseURL + "/models"
	KimiCodexAPIValidationTimeout  = 15 * time.Second
	KimiCodexAPIKeyEnvironment     = "KIMI_API_KEY"
	KimiCodexWireAPI               = "responses"
	KimiCodexCLIName               = "Kimi (Codex harness)"
	KimiCodexImageLabel            = "kimi-codex"
	KimiCodexCredentialName        = "kimi-codex"
	KimiCodexPersistentDevice      = "kimi-codex-home"
	KimiCodexHostDirectory         = "kimi-codex"
	KimiCodexAuthInstructions      = "Connect a Kimi Code API key to use Kimi models in project chats. K3 requires a Kimi plan with K3 access. This connection is separate from Kimi CLI device login."
	KimiCodexAPIKeyCreateURL       = "https://www.kimi.com/code/console"
	KimiCodexAPIKeyCreateLabel     = "Create a Kimi Code API key"
	KimiCodexAPIKeyCredentialLabel = "Kimi Code API key"

	KimiCodexContainerHome             = "/root/.kimi-codex"
	KimiCodexContainerCatalog          = KimiCodexContainerHome + "/model-catalog.json"
	KimiCodexContainerCatalogHash      = KimiCodexContainerHome + "/.model-catalog.sha256"
	KimiCodexContainerInstructions     = KimiCodexContainerHome + "/AGENTS.md"
	KimiCodexContainerInstructionsHash = KimiCodexContainerHome + "/.agents-md.sha256"
	KimiCodexContainerSkills           = KimiCodexContainerHome + "/skills"
	KimiCodexWorkspaceHome             = "/workspace/.kimi-codex"
)
