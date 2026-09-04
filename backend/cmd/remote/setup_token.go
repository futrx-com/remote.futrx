package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	service "github.com/futrx-com/remote.futrx.com/internal/service"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusers"
)

// runSetupToken reissues the first-boot setup token and prints it. It is
// reachable only from the server's own terminal, which is what keeps it from
// reopening the hole it exists to close: nothing web-facing can mint a token
// or ask for the current one to be shown again.
//
// Issuing rotates, so whatever was printed before stops working. That is the
// recovery path for an operator who lost the terminal or let the token expire.
//
// It asks the setup-token use case whether setup is still gated rather than
// deciding for itself, so this command and the running server cannot disagree
// about when a token is worth printing.
func runSetupToken(ctx context.Context, dataDir, baseURL string, setupTokenTTL time.Duration, out io.Writer) error {
	if _, err := serviceauth.NormalizeBaseURL(baseURL); err != nil {
		return err
	}
	authStore := fileauth.New(dataDir)
	usersStore, err := fileusers.New(dataDir)
	if err != nil {
		return fmt.Errorf("open user directory: %w", err)
	}
	setupTokens, err := service.NewSetupTokenIssuer(
		ctx,
		authStore,
		serviceuser.New(usersStore),
		setupTokenTTL,
	)
	if err != nil {
		return err
	}

	token, err := setupTokens.EnsureSetupToken(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		if setupTokens.LocalAdminConfigured() {
			return errors.New(
				"this server already has a local administrator; " +
					"remove DATA_DIR/local-admin.json on the host to start setup over",
			)
		}
		return errors.New(
			"this server already has an administrator, who sets the local password " +
				"themselves from Settings after signing in; no setup token is used",
		)
	}

	announceSetupTokenLink(out, baseURL, token, setupTokens.SetupTokenTTL())
	return nil
}
