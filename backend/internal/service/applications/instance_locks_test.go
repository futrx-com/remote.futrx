package applications

import "testing"

func TestReverseCallbackReadLockDoesNotWaitBehindLifecycle(t *testing.T) {
	var locks instanceLockSet
	unlock := locks.lock("instance")
	if read, ok := locks.tryRLock("instance"); ok {
		read()
		t.Fatal("callback entered during lifecycle mutation")
	}
	unlock()
	outer := locks.rlock("instance")
	defer outer()
	read, ok := locks.tryRLock("instance")
	if !ok {
		t.Fatal("callback cannot re-enter host-held read lock")
	}
	read()
}
