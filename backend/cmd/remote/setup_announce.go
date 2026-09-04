package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// setupTokenAnnouncer is the slice of the auth service that startup needs to
// decide whether to print a first-boot setup link.
type setupTokenAnnouncer interface {
	EnsureSetupToken(ctx context.Context) (string, error)
	SetupTokenTTL() time.Duration
}

// announceSetupToken prints the first-boot setup link when a claim would
// actually be gated on one, and stays quiet otherwise.
//
// A user directory it cannot read is a warning, not a reason to refuse to
// start. Deciding whether setup is pending means reading users.json, and
// before the token gate existed the server booted fine without ever doing so;
// making that read fatal would turn a damaged or unreadable users.json into an
// outage on the next restart. The operator gets told what actually broke and
// can reissue with `remote setup-token` once it is fixed.
func announceSetupToken(ctx context.Context, auth setupTokenAnnouncer, baseURL string, out io.Writer) {
	token, err := auth.EnsureSetupToken(ctx)
	if err != nil {
		fmt.Fprintf(out, "setup token: skipped, cannot read the user directory: %v\n", err)
		return
	}
	if token == "" {
		return
	}
	announceSetupTokenLink(out, baseURL, token, auth.SetupTokenTTL())
}

// announceSetupTokenLink formats the one line an operator acts on.
func announceSetupTokenLink(out io.Writer, baseURL, token string, ttl time.Duration) {
	fmt.Fprintf(out,
		"first-time setup required\n  visit:   %s/?token=%s\n  expires: %s from now (reissue with: remote setup-token)\n",
		strings.TrimRight(baseURL, "/"), token, ttl,
	)
}
