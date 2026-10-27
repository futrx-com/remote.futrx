package constants

import "time"

// Kimi's process and protocol limits are kept distinct even where values match.
const (
	KimiServerStartupTimeout  = 30 * time.Second
	KimiServerRequestTimeout  = 30 * time.Second
	KimiServerShutdownTimeout = 5 * time.Second
	KimiServerStderrTailBytes = 64 << 10
	KimiBridgeScanBufferBytes = 64 << 10
	KimiBridgeMaxFrameBytes   = 16 << 20
	KimiBridgeFrameQueueSize  = 64
	KimiBridgeCloseTimeout    = 8 * time.Second
	KimiBridgeKillTimeout     = 2 * time.Second
	KimiRunIdlePollInterval   = 250 * time.Millisecond
	KimiRunAbortTimeout       = 4 * time.Second
	KimiChildDeltaInterval    = 100 * time.Millisecond
)
