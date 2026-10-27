package capability

import (
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// catalogCache is the shared freshness boundary for every web client. Entries
// are keyed by the host or by project ID plus container name and expire lazily
// on the next read. The cache is intentionally process-local: restarting or
// deploying the backend forces one fresh discovery per requested environment.
type catalogCache struct {
	mu          sync.Mutex
	now         func() time.Time
	liveTTL     time.Duration
	degradedTTL time.Duration
	entries     map[string]catalogCacheEntry
}

type catalogCacheEntry struct {
	expiresAt time.Time
	result    []agent.Capabilities
}

func newCatalogCache(liveTTL, degradedTTL time.Duration) *catalogCache {
	return &catalogCache{
		now:         time.Now,
		liveTTL:     liveTTL,
		degradedTTL: degradedTTL,
		entries:     make(map[string]catalogCacheEntry),
	}
}

func (c *catalogCache) load(key string) ([]agent.Capabilities, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return cloneCapabilities(entry.result), true
}

func (c *catalogCache) store(key string, result []agent.Capabilities) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = catalogCacheEntry{
		expiresAt: c.now().Add(c.ttl(result)),
		result:    cloneCapabilities(result),
	}
}

func (c *catalogCache) ttl(result []agent.Capabilities) time.Duration {
	for _, capabilities := range result {
		if capabilities.Source != agent.CapabilitySourceLive || capabilities.Warning != "" {
			return c.degradedTTL
		}
	}
	return c.liveTTL
}
