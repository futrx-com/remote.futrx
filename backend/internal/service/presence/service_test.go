package presence

import (
	"testing"
	"time"
)

// at builds a service reading a clock the test drives, so claims can be aged
// out without sleeping through the real TTL.
func at(now *time.Time) *Service {
	return newAt(func() time.Time { return *now })
}

func TestAClaimHoldsUntilItExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})

	if !service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("a fresh claim should mark the user as watching")
	}

	now = now.Add(TTL - time.Second)
	if !service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("a claim inside the TTL should still hold")
	}

	now = now.Add(2 * time.Second)
	if service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("a claim past the TTL should be ignored")
	}
}

func TestAClaimIsScopedToTheChatAndUser(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})

	if service.IsWatching("dev@example.com", "chat-2") {
		t.Fatal("watching one chat must not silence another")
	}
	if service.IsWatching("other@example.com", "chat-1") {
		t.Fatal("one user's claim must not silence another user")
	}
}

// The subscription store keys on a lowercased email, so a claim made under a
// differently-cased address has to resolve to the same user or the filter
// silently stops matching.
func TestClaimsIgnoreEmailCaseAndSurroundingSpace(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("  Dev@Example.COM ", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})

	if !service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("presence should key on the same normalized email push does")
	}
}

func TestReleaseWithdrawsAClaimImmediately(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})
	service.Record("dev@example.com", Report{ClientID: "tab-1", Revision: 2})

	if service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("a released claim should not survive its TTL")
	}
}

// A background tab signing off must not cancel the focused tab's claim,
// whichever order the two requests land in.
func TestClientsClaimIndependently(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("dev@example.com", Report{ClientID: "focused", ChatID: "chat-1", Revision: 1})
	service.Record("dev@example.com", Report{ClientID: "background", Revision: 1})

	if !service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("one client going away should leave another client's claim standing")
	}
}

// A caller that sends no client id is still tracked, as a single implicit
// client, rather than silently failing to register a claim.
func TestAMissingClientIDStillClaims(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("dev@example.com", Report{ChatID: "chat-1", Revision: 1})

	if !service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("an absent client id should fall back to one implicit client")
	}

	service.Record("dev@example.com", Report{Revision: 2})
	if service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("that implicit client should be releasable the same way")
	}
}

func TestFilterRemovesOnlyWatchers(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	service.Record("watcher@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})
	service.Record("elsewhere@example.com", Report{ClientID: "tab-1", ChatID: "chat-2", Revision: 1})

	kept := service.Filter(
		[]string{"watcher@example.com", "elsewhere@example.com", "away@example.com"},
		"chat-1",
	)

	want := []string{"elsewhere@example.com", "away@example.com"}
	if len(kept) != len(want) {
		t.Fatalf("kept %v, want %v", kept, want)
	}
	for i, email := range want {
		if kept[i] != email {
			t.Fatalf("kept %v, want %v", kept, want)
		}
	}
}

// A deployment that never wired presence up must notify exactly as before.
func TestNilServiceFiltersNothing(t *testing.T) {
	var service *Service
	recipients := []string{"dev@example.com"}

	if got := service.Filter(recipients, "chat-1"); len(got) != 1 {
		t.Fatalf("nil presence should keep every recipient, got %v", got)
	}
	if service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("nil presence should never report a watcher")
	}
	// Neither entry point may panic on a service that was never built.
	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})
	service.Record("dev@example.com", Report{ClientID: "tab-1", Revision: 2})
}

func TestStaleClientsAreEvictedRatherThanAccumulating(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)
	for i := 0; i < maxClientsPerUser*2; i++ {
		service.Record("dev@example.com", Report{
			ClientID: string(rune('a'+i%26)) + string(rune('0'+i/26)),
			ChatID:   "chat-1",
			Revision: 1,
		})
		now = now.Add(time.Second)
	}

	service.mu.Lock()
	tracked := len(service.users["dev@example.com"].byClient)
	service.mu.Unlock()
	if tracked > maxClientsPerUser {
		t.Fatalf("tracked %d clients for one user, cap is %d", tracked, maxClientsPerUser)
	}
}

func TestDelayedClaimCannotOverrideANewerRelease(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)

	// The release reaches the server first; the earlier claim arrives late.
	service.Record("dev@example.com", Report{ClientID: "tab-1", Revision: 2})
	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 1})

	if service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("a delayed claim revived a client after its newer release")
	}
}

func TestNewestPresenceRevisionWinsAcrossChatChanges(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	service := at(&now)

	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-2", Revision: 3})
	service.Record("dev@example.com", Report{ClientID: "tab-1", ChatID: "chat-1", Revision: 2})

	if service.IsWatching("dev@example.com", "chat-1") {
		t.Fatal("an older claim replaced the current chat")
	}
	if !service.IsWatching("dev@example.com", "chat-2") {
		t.Fatal("the newest claim was not retained")
	}
}
