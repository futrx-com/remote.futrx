// Package constants contains application-wide configuration values.
package constants

const (
	// DefaultChatTranscriptTurnLimit applies when no positive limit is requested.
	DefaultChatTranscriptTurnLimit = 20
	// MaxChatTranscriptTurnLimit caps transcript page sizes.
	MaxChatTranscriptTurnLimit = 100
	// DefaultChatTranscriptByteLimit bounds a history response independently
	// of turn size. Large turns are continued with the existing sequence cursor.
	DefaultChatTranscriptByteLimit = 4 * 1024 * 1024
	// MaxChatTranscriptByteLimit prevents callers from turning one history read
	// back into an unbounded allocation.
	MaxChatTranscriptByteLimit = 8 * 1024 * 1024
	// ChatTranscriptInlineFieldBytes bounds oversized fields in stored previews.
	ChatTranscriptInlineFieldBytes = 32 * 1024
	// DefaultChatTranscriptContentPageBytes sizes lazy full-content reads.
	DefaultChatTranscriptContentPageBytes = 256 * 1024
	// MaxChatTranscriptContentPageBytes caps a single full-content chunk.
	MaxChatTranscriptContentPageBytes = 1024 * 1024
	// ChatIndexCheckpointBytes bounds index backfill batches before advancing
	// to the next complete event record.
	ChatIndexCheckpointBytes int64 = 32 * 1024 * 1024
	// StartupChatIndexWarmupChatLimit bounds the recent chats indexed during
	// startup. It counts chats, not transcript turns.
	StartupChatIndexWarmupChatLimit = 10
	// PromptInteractionResponseQueueCapacity bounds browser answers waiting for
	// the active provider turn to consume them.
	PromptInteractionResponseQueueCapacity = 64
)
