package constants

import "time"

const (
	MiniMaxLabel                        = "MiniMax"
	MiniMaxAutoModelLabel               = "MiniMax default"
	MiniMaxDefaultModelContextWindow    = 204_800
	MiniMaxLargeModelContextWindow      = 1_000_000
	MiniMaxAPIBaseURL                   = "https://api.minimax.io/v1"
	MiniMaxModelsURL                    = MiniMaxAPIBaseURL + "/models"
	MiniMaxAPIValidationTimeout         = 10 * time.Second
	MiniMaxAPIKeyEnvironment            = "MINIMAX_API_KEY"
	MiniMaxWireAPI                      = "responses"
	MiniMaxReasoningDisabled            = "none"
	MiniMaxReasoningDisabledLabel       = "Think-Off"
	MiniMaxReasoningDisabledDescription = "Disable Adaptive Thinking"
	MiniMaxReasoningAdaptive            = "high"
	MiniMaxReasoningAdaptiveLabel       = "Adaptive"
	MiniMaxReasoningAdaptiveDescription = "Enable Adaptive Thinking"
	MiniMaxCLIName                      = "MiniMax (Codex harness)"
	MiniMaxImageLabel                   = "minimax"
	MiniMaxCredentialName               = "minimax"
	MiniMaxPersistentDevice             = "minimax-home"
	MiniMaxHostDirectory                = "minimax"
	MiniMaxAuthInstructions             = "MiniMax is available only with a Token Plan subscription. Add a Token Plan subscription key to use it in project chats; pay-as-you-go API keys are not supported."
	MiniMaxAPIKeyCreateURL              = "https://platform.minimax.io/subscribe/token-plan"
	MiniMaxAPIKeyCreateLabel            = "Get a MiniMax Token Plan subscription key"
	MiniMaxAPIKeyCredentialLabel        = "MiniMax Token Plan subscription key"
	MiniMaxTokenPlanKeyPrefix           = "sk-cp-"
	MiniMaxTokenPlanValidationURL       = MiniMaxAPIBaseURL + "/token_plan/remains"

	MiniMaxContainerHome             = "/root/.minimax"
	MiniMaxContainerCatalog          = MiniMaxContainerHome + "/model-catalog.json"
	MiniMaxContainerCatalogHash      = MiniMaxContainerHome + "/.model-catalog.sha256"
	MiniMaxContainerInstructions     = MiniMaxContainerHome + "/AGENTS.md"
	MiniMaxContainerInstructionsHash = MiniMaxContainerHome + "/.agents-md.sha256"
	MiniMaxContainerSkills           = MiniMaxContainerHome + "/skills"
	MiniMaxWorkspaceHome             = "/workspace/.minimax"
)
