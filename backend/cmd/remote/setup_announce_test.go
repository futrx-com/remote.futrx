package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type announcerStub struct {
	token string
	err   error
}

func (s announcerStub) EnsureSetupToken(context.Context) (string, error) { return s.token, s.err }
func (s announcerStub) SetupTokenTTL() time.Duration                     { return 30 * time.Minute }

func TestAnnounceSetupTokenPrintsTheQueryLink(t *testing.T) {
	var out strings.Builder
	announceSetupToken(context.Background(), announcerStub{token: "abc123"}, "https://remote.example.com/", &out)

	printed := out.String()
	if !strings.Contains(printed, "https://remote.example.com/?token=abc123") {
		t.Fatalf("printed output lacks the query link:\n%s", printed)
	}
}

func TestAnnounceSetupTokenSaysNothingWhenNoneIsNeeded(t *testing.T) {
	var out strings.Builder
	announceSetupToken(context.Background(), announcerStub{token: ""}, "https://remote.example.com", &out)

	if out.Len() != 0 {
		t.Fatalf("printed a setup message for a server that needs none: %s", out.String())
	}
}

// An unreadable or corrupt users.json must not stop the server booting. It did
// not before the token gate existed, and a restart turning a damaged directory
// into an outage is worse than starting without a setup link.
func TestAnnounceSetupTokenWarnsRatherThanFailingOnADirectoryError(t *testing.T) {
	var out strings.Builder
	announceSetupToken(
		context.Background(),
		announcerStub{err: errors.New("parse users.json: invalid character 'T'")},
		"https://remote.example.com",
		&out,
	)

	printed := out.String()
	if !strings.Contains(printed, "cannot read the user directory") {
		t.Fatalf("warning does not name the real problem:\n%s", printed)
	}
	if !strings.Contains(printed, "parse users.json") {
		t.Fatalf("warning drops the underlying cause:\n%s", printed)
	}
	if strings.Contains(printed, "?token=") {
		t.Fatalf("printed a setup link despite being unable to decide:\n%s", printed)
	}
}
