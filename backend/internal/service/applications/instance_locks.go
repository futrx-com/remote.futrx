package applications

import "sync"

// instanceLockSet serializes lifecycle and lazy-backend work per installed
// copy without making an unrelated application wait. Entries are reference
// counted so instance IDs do not accumulate for the lifetime of the server
// after uninstall or ordinary one-off calls.
type instanceLockSet struct {
	mu      sync.Mutex
	entries map[string]*instanceLock
}

type instanceLock struct {
	mu   sync.RWMutex
	refs int
}

// lock blocks until the caller exclusively owns id and returns its release
// function. The zero value is ready for use.
func (s *instanceLockSet) lock(id string) func() {
	entry := s.retain(id)
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		s.release(id, entry)
	}
}

// rlock lets backend calls and event notifications for one running instance
// overlap while still excluding lifecycle and reconfiguration work.
func (s *instanceLockSet) rlock(id string) func() {
	entry := s.retain(id)
	entry.mu.RLock()
	return func() {
		entry.mu.RUnlock()
		s.release(id, entry)
	}
}

func (s *instanceLockSet) retain(id string) *instanceLock {
	s.mu.Lock()
	if s.entries == nil {
		s.entries = make(map[string]*instanceLock)
	}
	entry := s.entries[id]
	if entry == nil {
		entry = &instanceLock{}
		s.entries[id] = entry
	}
	entry.refs++
	s.mu.Unlock()
	return entry
}

func (s *instanceLockSet) release(id string, entry *instanceLock) {
	s.mu.Lock()
	entry.refs--
	if entry.refs == 0 && s.entries[id] == entry {
		delete(s.entries, id)
	}
	s.mu.Unlock()
}
