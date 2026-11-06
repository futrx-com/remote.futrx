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
	// MaxArchiveBytes matches the container's archive spool. The whole ZIP is
	// held in memory to answer one backend call.
	MaxArchiveBytes int64 = 256 << 20
	// MaxConcurrentArchives bounds the archives one project builds at once.
	MaxConcurrentArchives = 2
)
