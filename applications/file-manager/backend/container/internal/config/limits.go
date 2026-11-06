package config

const (
	WorkspaceDirectory = "/workspace"

	MaxDirectoryEntries          = 10000
	MaxSearchResults             = 300
	MinSearchQuery               = 2
	MaxSearchVisits              = 200000
	MaxArchiveSourceBytes  int64 = 1 << 30
	MaxArchiveEntries            = 200000
	DirectoryReadBatchSize       = 256
	// The host backend holds a finished archive in memory to answer one
	// request, so the compressed size stays well below the source budget.
	MaxArchiveSpoolBytes int64 = 256 << 20
	// MaxRequestBytes bounds the JSON request the host writes to stdin.
	MaxRequestBytes = 64 << 10
)
