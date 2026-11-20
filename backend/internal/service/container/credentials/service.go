// Package credentials coordinates provider credential synchronization without
// depending on LXD commands or host-filesystem implementation details.
package credentials

import (
	"context"
	"errors"
	"fmt"

	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// Transfer performs the LXD and host-filesystem mechanics for each supported
// credential shape. Service owns the profile policy that selects the shape.
type Transfer interface {
	EnsureFiles(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error
	EnsureDirectory(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error
	SyncFilesFromContainer(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error
	SyncDirectoryFromContainer(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error
}

// Service owns credential profile selection and synchronization orchestration.
type Service struct {
	profiles serviceprofiles.Source
	transfer Transfer
}

func NewService(profileSource serviceprofiles.Source, transfer Transfer) *Service {
	return &Service{
		profiles: profileSource,
		transfer: transfer,
	}
}

// EnsureRegistered seeds every profile that opts into launch-time credential
// provisioning. Errors are joined so one provider does not hide another
// provider's failure.
func (s *Service) EnsureRegistered(ctx context.Context, containerName string) error {
	var errs []error
	for _, profile := range s.profiles.Snapshot() {
		credentials := profile.Credentials
		if credentials.Empty() || !credentials.SeedOnLaunch {
			continue
		}
		if err := s.Ensure(ctx, containerName, credentials); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", credentials.Name, err))
		}
	}
	return errors.Join(errs...)
}

// Ensure pushes a profile's credentials into a container through the adapter
// for that profile's credential shape.
func (s *Service) Ensure(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	if spec.Directory != nil {
		return s.transfer.EnsureDirectory(ctx, containerName, spec)
	}
	return s.transfer.EnsureFiles(ctx, containerName, spec)
}

// SyncFromContainer pulls credentials back to the host through the adapter for
// that profile's credential shape.
func (s *Service) SyncFromContainer(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	if spec.Directory != nil {
		return s.transfer.SyncDirectoryFromContainer(ctx, containerName, spec)
	}
	return s.transfer.SyncFilesFromContainer(ctx, containerName, spec)
}
