package config

import "time"

const (
	WorkspaceDirectory = "/workspace"

	MaxDirectoryEntries          = 10000
	MaxSearchResults             = 300
	MinSearchQuery               = 2
	MaxSearchVisits              = 200000
	MaxArchiveSourceBytes  int64 = 1 << 30
	MaxArchiveEntries            = 200000
	DirectoryReadBatchSize       = 256
	// MaxArchiveBytes bounds the disk one finished ZIP uses. Stored entries and
	// headers can make a ZIP slightly larger than its sources.
	MaxArchiveBytes int64 = MaxArchiveSourceBytes + 64<<20
	// MaxRequestBytes bounds the JSON request the host writes to stdin.
	MaxRequestBytes = 64 << 10

	// DownloadsDirectory holds finished folder ZIPs, relative to the workspace,
	// so Remote's chat download route can stream them straight to the browser.
	// Listings, searches and archives leave it out.
	DownloadsDirectory = ".file-manager-downloads"
	// DownloadRetention is how long a finished ZIP waits before the next
	// archive build deletes it. The browser starts fetching it immediately.
	DownloadRetention = time.Hour
)
