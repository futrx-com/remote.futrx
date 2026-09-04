package auth

import (
	"context"
	"errors"
	"testing"
)

const claimTestPassword = "correct horse battery staple"

// The vulnerability this gate closes: on a genuinely first boot the user
// directory is empty, so nobody exists to authorise a claim and whoever
// reached the page first became the owner.
func TestClaimRejectedWithoutSetupTokenOnFirstBoot(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	issueSetupTokenForTest(t, service)

	_, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "attacker@example.com", Password: claimTestPassword,
	})
	if !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("tokenless claim error = %v, want ErrSetupTokenRequired", err)
	}
	if store.local != nil || service.LocalAdminConfigured() {
		t.Fatal("a tokenless claim wrote the local admin credential")
	}
	if len(users.roles) != 0 {
		t.Fatalf("a tokenless claim created users: %#v", users.roles)
	}
}

func TestClaimRejectedWithWrongSetupToken(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	issueSetupTokenForTest(t, service)

	_, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "attacker@example.com", Password: claimTestPassword,
		SetupToken: "guessed-token",
	})
	if !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("wrong-token claim error = %v, want ErrSetupTokenRequired", err)
	}
	if store.local != nil {
		t.Fatal("a wrong-token claim wrote the local admin credential")
	}
}

// A token works exactly once. The second attempt is refused before expiry,
// so a token scraped from scrollback or a proxy log is worthless.
func TestClaimTokenIsSpentByASuccessfulClaim(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	token := issueSetupTokenForTest(t, service)

	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: claimTestPassword, SetupToken: token,
	}); err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}
	if store.setupToken == nil || !store.setupToken.Used {
		t.Fatalf("token record after a successful claim = %#v, want Used", store.setupToken)
	}
}

// Once the server is claimed, no token - not even a freshly issued one -
// may overwrite the credential.
func TestClaimAfterSetupIsRefusedEvenWithAFreshToken(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	token := issueSetupTokenForTest(t, service)

	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: claimTestPassword, SetupToken: token,
	}); err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}
	claimed := *store.local

	fresh := issueSetupTokenForTest(t, service)
	_, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "attacker@example.com", Password: "another secure password",
		SetupToken: fresh,
	})
	if !errors.Is(err, ErrLocalAdminAlreadyClaimed) {
		t.Fatalf("second claim error = %v, want ErrLocalAdminAlreadyClaimed", err)
	}
	if *store.local != claimed {
		t.Fatalf("credential was overwritten: %#v", store.local)
	}
}

// A claim can still fail after the token checks out. Spending it there would
// leave the operator with no credential and no way back in short of the
// terminal, so the token must survive every recoverable failure.
func TestClaimFailureLeavesTheSetupTokenSpendable(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	token := issueSetupTokenForTest(t, service)

	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: "short", SetupToken: token,
	}); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("weak-password claim error = %v, want ErrPasswordTooShort", err)
	}
	if store.setupToken == nil || store.setupToken.Used {
		t.Fatalf("token record after a failed claim = %#v, want unused", store.setupToken)
	}

	// The same token still completes the claim the operator meant to make.
	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: claimTestPassword, SetupToken: token,
	}); err != nil {
		t.Fatalf("retry with the same token: %v", err)
	}
}

// Once an administrator exists they authorise the claim themselves. Requiring
// a terminal token there would add friction for a caller who is already
// authenticated, so that path is deliberately untouched.
func TestClaimByExistingAdminNeedsNoSetupToken(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	users.roles["admin@example.com"] = true
	service := newAuthTestService(t, store, users, User{})

	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: claimTestPassword,
		AuthorizedEmail: "admin@example.com",
	}); err != nil {
		t.Fatalf("administrator-authorised claim: %v", err)
	}
}

// An unreadable token file must not read as "no gate in place".
func TestClaimFailsClosedWhenTokenStoreIsUnreadable(t *testing.T) {
	store := &authTestStore{setupTokenErr: errors.New("permission denied")}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})

	_, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "attacker@example.com", Password: claimTestPassword,
		SetupToken: "anything",
	})
	if !errors.Is(err, ErrSetupTokenUnavailable) {
		t.Fatalf("claim with unreadable token store = %v, want ErrSetupTokenUnavailable", err)
	}
	if store.local != nil {
		t.Fatal("a claim succeeded while the token gate was unreadable")
	}
}

// Startup calls EnsureSetupToken on every boot. While unclaimed that rotates
// the token, so one printed before a restart is already dead.
func TestEnsureSetupTokenRotatesOnEveryUnclaimedStart(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})

	first, err := service.EnsureSetupToken(context.Background())
	if err != nil || first == "" {
		t.Fatalf("first EnsureSetupToken = %q, %v", first, err)
	}
	second, err := service.EnsureSetupToken(context.Background())
	if err != nil || second == "" {
		t.Fatalf("second EnsureSetupToken = %q, %v", second, err)
	}
	if first == second {
		t.Fatal("a restart reused the previous setup token instead of rotating it")
	}

	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: claimTestPassword, SetupToken: first,
	}); !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("claim with the pre-restart token = %v, want ErrSetupTokenRequired", err)
	}
}

// A configured server must never print a setup URL again, which would both
// confuse the operator and put a live token in the log of a running system.
func TestEnsureSetupTokenIssuesNothingOnceClaimed(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	token := issueSetupTokenForTest(t, service)

	if _, err := service.ClaimLocalAdmin(context.Background(), ClaimRequest{
		Email: "admin@example.com", Password: claimTestPassword, SetupToken: token,
	}); err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}

	issued, err := service.EnsureSetupToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureSetupToken after claim: %v", err)
	}
	if issued != "" {
		t.Fatal("a claimed server issued a fresh setup token on restart")
	}
}

// An unclaimed server whose directory already has an administrator is not
// token-gated: that administrator authorises the claim. Printing a setup URL
// there sends the operator down a path the token can never complete.
func TestEnsureSetupTokenIssuesNothingWhenAnAdministratorExists(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	users.roles["googleadmin@example.com"] = true
	service := newAuthTestService(t, store, users, User{})

	issued, err := service.EnsureSetupToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}
	if issued != "" {
		t.Fatalf("issued a setup token = %q for a claim an administrator authorises", issued)
	}
	if store.setupToken != nil {
		t.Fatal("a token record was written for a claim that is not token-gated")
	}
}
