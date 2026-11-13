package config

import "time"

const (
	// ContainerCommand is the program backend/container/ builds into the
	// project container. Every request runs it once through `lxc exec`.
	ContainerCommand = "/usr/local/bin/file-manager"

	ListTimeout    = 30 * time.Second
	ArchiveTimeout = 4 * time.Minute

	// MaxQueryBytes bounds a path or search term taken from the browser.
	MaxQueryBytes = 4096
	// MaxListingBytes bounds the JSON a listing or search may return.
	MaxListingBytes int64 = 32 << 20
	// MaxArchiveResultBytes bounds the JSON an archive build returns: only the
	// finished ZIP's path, never its bytes.
	MaxArchiveResultBytes int64 = 64 << 10
	// DownloadsDirectory mirrors the container's: finished ZIPs wait there,
	// relative to the workspace, for Remote's chat download route.
	DownloadsDirectory = ".file-manager-downloads"
	// MaxConcurrentArchives bounds the archives one project builds at once.
	MaxConcurrentArchives = 2
)
