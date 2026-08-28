package auth

import "sync"

// keyedMutex hands out one mutex per account email, created on first use. A
// caller holds a key's lock exclusively; different accounts never block each
// other.
//
// Both twoFactorAuthenticator and sessionRegistry read a record, decide
// against it, and write it back. The cache RWMutex only guards the map
// itself, so without this an account's read-modify-write can interleave with
// its own concurrent copy - most importantly letting two racing logins each
// redeem the same single-use recovery code.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// lock blocks until the caller owns email's mutex and returns its unlock func.
func (k *keyedMutex) lock(email string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*sync.Mutex)
	}
	mu := k.locks[email]
	if mu == nil {
		mu = &sync.Mutex{}
		k.locks[email] = mu
	}
	k.mu.Unlock()
	mu.Lock()
	return mu.Unlock
}
