package project

import "sync"

// keyedMutex hands out one mutex per key, created on first use. A caller holds
// a key's lock exclusively; different keys never block each other.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[ID]*sync.Mutex
}

// lock blocks until the caller owns key's mutex and returns its unlock func.
func (k *keyedMutex) lock(key ID) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[ID]*sync.Mutex)
	}
	mu := k.locks[key]
	if mu == nil {
		mu = &sync.Mutex{}
		k.locks[key] = mu
	}
	k.mu.Unlock()
	mu.Lock()
	return mu.Unlock
}
