package pluginhost

import "sync"

// keyedLocks serializes work per key without serializing unrelated keys. Both
// building an image and launching an instance are expensive and idempotent, so
// concurrent callers should wait for one another rather than duplicate the
// work — but a slow plugin must not hold up every other plugin.
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedLocks() keyedLocks {
	return keyedLocks{locks: map[string]*sync.Mutex{}}
}

// lock acquires the mutex for key and returns its release function.
func (k *keyedLocks) lock(key string) func() {
	k.mu.Lock()
	entry, ok := k.locks[key]
	if !ok {
		entry = &sync.Mutex{}
		k.locks[key] = entry
	}
	k.mu.Unlock()

	entry.Lock()
	return entry.Unlock
}
