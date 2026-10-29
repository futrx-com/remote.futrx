package config

const (
	MaxDirectoryEntries          = 10000
	MaxSearchResults             = 300
	MinSearchQuery               = 2
	MaxSearchVisits              = 200000
	MaxArchiveSourceBytes  int64 = 1 << 30
	MaxArchiveEntries            = 200000
	DirectoryReadBatchSize       = 256
	MaxArchiveSpoolBytes   int64 = 1 << 30
	MaxConcurrentArchives        = 2
)
