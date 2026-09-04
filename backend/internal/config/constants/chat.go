// Package constants contains application-wide configuration values.
package constants

const (
	// DefaultChatTranscriptTurnLimit applies when no positive limit is requested.
	DefaultChatTranscriptTurnLimit = 20
	// MaxChatTranscriptTurnLimit caps transcript page sizes.
	MaxChatTranscriptTurnLimit = 100
	// PromptInteractionResponseQueueCapacity bounds browser answers waiting for
	// the active provider turn to consume them.
	PromptInteractionResponseQueueCapacity = 64
)
