// Package profiles owns the immutable agent-profile catalog consumed by
// container application services and adapters.
package profiles

import "github.com/futrx-com/remote.futrx.com/internal/harness/provisioning"

// Source exposes defensive snapshots of the configured agent profiles.
type Source interface {
	Snapshot() []provisioning.Profile
}

// Catalog is an immutable, defensively copied profile source.
type Catalog struct {
	profiles []provisioning.Profile
}

func NewCatalog(configured []provisioning.Profile) *Catalog {
	return &Catalog{profiles: clone(configured)}
}

func (c *Catalog) Snapshot() []provisioning.Profile {
	return clone(c.profiles)
}

func clone(profiles []provisioning.Profile) []provisioning.Profile {
	out := make([]provisioning.Profile, len(profiles))
	for index := range profiles {
		out[index] = profiles[index].Clone()
	}
	return out
}
