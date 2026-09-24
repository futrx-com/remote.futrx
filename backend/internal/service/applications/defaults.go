package applications

import (
	"context"
	"errors"
	"fmt"
)

// builtInDefaultApplicationIDs is the product policy for applications a
// server installs globally on the first startup that knows about them. Keep
// this list to built-in catalog IDs only.
var builtInDefaultApplicationIDs = []string{
	"file-management",
}

// DefaultApplicationIDs returns a copy of the product's default-application
// list for composition into the applications service.
func DefaultApplicationIDs() []string {
	return append([]string(nil), builtInDefaultApplicationIDs...)
}

// ReconcileDefaultApplications installs each newly introduced default once.
// A durable marker is written only after installation succeeds. Existing
// running or stopped global instances are adopted without changing their
// state, which preserves an administrator's explicit stop. Once marked, an
// absent instance is also left absent, which preserves an explicit uninstall.
func (s *Service) ReconcileDefaultApplications(ctx context.Context) error {
	if len(s.defaultIDs) == 0 {
		return nil
	}
	if s.defaultStore == nil {
		return errors.New("applications: default installation store is unavailable")
	}

	installed, err := s.defaultStore.ListDefaultInstallations(ctx)
	if err != nil {
		// Without the marker set there is no safe way to distinguish a fresh
		// server from an administrator who deliberately uninstalled a default.
		return fmt.Errorf("applications: read default installations: %w", err)
	}
	marked := make(map[string]bool, len(installed))
	for _, id := range installed {
		marked[id] = true
	}

	var errs []error
	seen := make(map[string]bool, len(s.defaultIDs))
	for _, id := range s.defaultIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := s.reconcileDefaultApplication(ctx, id, marked[id]); err != nil {
			errs = append(errs, fmt.Errorf("default application %q: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) reconcileDefaultApplication(ctx context.Context, id string, marked bool) error {
	application, ok := s.registry.Get(id)
	if !ok {
		return ErrUnknownApplication
	}
	if application.Source != SourceBuiltin {
		return fmt.Errorf("%w: source is %q, want %q", ErrInvalidDefault, application.Source, SourceBuiltin)
	}
	if !application.SupportsScope(ScopeGlobal) {
		return fmt.Errorf("%w: global scope is not supported", ErrInvalidDefault)
	}
	if marked {
		return nil
	}

	existing, found, err := s.instanceOfApplication(ctx, ScopeGlobal, "", id)
	if err != nil {
		return err
	}
	if found {
		switch existing.Status {
		case StatusRunning, StatusStopped:
			return s.markDefaultInstallation(ctx, id)
		case StatusError:
			// Install owns cleanup-and-retry for failed attempts.
		case StatusInstalling:
			// Reconciliation runs once before HTTP startup, so an installing
			// record here was left by a process that stopped mid-install. Clear
			// that partial attempt before installing it from scratch.
			if err := s.Uninstall(ctx, existing.ID); err != nil {
				return fmt.Errorf("remove interrupted installation: %w", err)
			}
		default:
			return fmt.Errorf("%w: unexpected status %q", ErrInvalidState, existing.Status)
		}
	}

	if _, err := s.Install(ctx, InstallRequest{
		ApplicationID: id,
		Scope:         ScopeGlobal,
	}); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return s.markDefaultInstallation(ctx, id)
}

func (s *Service) markDefaultInstallation(ctx context.Context, id string) error {
	if err := s.defaultStore.MarkDefaultInstallation(ctx, id); err != nil {
		return fmt.Errorf("record installation: %w", err)
	}
	return nil
}
