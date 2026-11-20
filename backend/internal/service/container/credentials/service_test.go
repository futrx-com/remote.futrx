package credentials

import (
	"context"
	"errors"
	"reflect"
	"testing"

	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

type recordingTransfer struct {
	calls []string
	errs  map[string]error
}

func (t *recordingTransfer) record(operation, containerName string, spec provisioning.CredentialSpec) error {
	call := operation + " " + containerName + " " + spec.Name
	t.calls = append(t.calls, call)
	return t.errs[call]
}

func (t *recordingTransfer) EnsureFiles(_ context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return t.record("ensure files", containerName, spec)
}

func (t *recordingTransfer) EnsureDirectory(_ context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return t.record("ensure directory", containerName, spec)
}

func (t *recordingTransfer) SyncFilesFromContainer(_ context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return t.record("sync files", containerName, spec)
}

func (t *recordingTransfer) SyncDirectoryFromContainer(_ context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return t.record("sync directory", containerName, spec)
}

func TestEnsureRegisteredKeepsProfileOrderAndJoinsNamedErrors(t *testing.T) {
	alphaErr := errors.New("alpha failed")
	betaErr := errors.New("beta failed")
	transfer := &recordingTransfer{errs: map[string]error{
		"ensure files c1 alpha":    alphaErr,
		"ensure directory c1 beta": betaErr,
	}}
	catalog := serviceprofiles.NewCatalog([]provisioning.Profile{
		{Credentials: provisioning.CredentialSpec{Name: "empty", SeedOnLaunch: true}},
		{Credentials: provisioning.CredentialSpec{
			Name:  "not-seeded",
			Files: []provisioning.CredentialFile{{HostPath: "ignored"}},
		}},
		{Credentials: provisioning.CredentialSpec{
			Name:         "alpha",
			SeedOnLaunch: true,
			Files:        []provisioning.CredentialFile{{HostPath: "alpha.json"}},
		}},
		{Credentials: provisioning.CredentialSpec{
			Name:         "beta",
			SeedOnLaunch: true,
			Directory:    &provisioning.CredentialDirectory{HostPath: "beta"},
		}},
	})

	err := NewService(catalog, transfer).EnsureRegistered(context.Background(), "c1")

	wantCalls := []string{"ensure files c1 alpha", "ensure directory c1 beta"}
	if !reflect.DeepEqual(transfer.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", transfer.calls, wantCalls)
	}
	if err == nil || err.Error() != "alpha: alpha failed\nbeta: beta failed" {
		t.Fatalf("EnsureRegistered error = %v, want joined named errors", err)
	}
	if !errors.Is(err, alphaErr) || !errors.Is(err, betaErr) {
		t.Fatalf("EnsureRegistered error = %v, want both wrapped causes", err)
	}
}

func TestEnsureAndSyncDispatchByCredentialShapeWithoutRewrapping(t *testing.T) {
	fileEnsureErr := errors.New("ensure files failed")
	directoryEnsureErr := errors.New("ensure directory failed")
	fileSyncErr := errors.New("sync files failed")
	directorySyncErr := errors.New("sync directory failed")
	transfer := &recordingTransfer{errs: map[string]error{
		"ensure files c1 files":         fileEnsureErr,
		"ensure directory c1 directory": directoryEnsureErr,
		"sync files c1 files":           fileSyncErr,
		"sync directory c1 directory":   directorySyncErr,
	}}
	service := NewService(serviceprofiles.NewCatalog(nil), transfer)
	files := provisioning.CredentialSpec{Name: "files"}
	directory := provisioning.CredentialSpec{
		Name:      "directory",
		Directory: &provisioning.CredentialDirectory{},
	}

	checks := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "ensure files", run: func() error {
			return service.Ensure(context.Background(), "c1", files)
		}, want: fileEnsureErr},
		{name: "ensure directory", run: func() error {
			return service.Ensure(context.Background(), "c1", directory)
		}, want: directoryEnsureErr},
		{name: "sync files", run: func() error {
			return service.SyncFromContainer(context.Background(), "c1", files)
		}, want: fileSyncErr},
		{name: "sync directory", run: func() error {
			return service.SyncFromContainer(context.Background(), "c1", directory)
		}, want: directorySyncErr},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if got := check.run(); got != check.want {
				t.Fatalf("error = %v, want original %v", got, check.want)
			}
		})
	}

	wantCalls := []string{
		"ensure files c1 files",
		"ensure directory c1 directory",
		"sync files c1 files",
		"sync directory c1 directory",
	}
	if !reflect.DeepEqual(transfer.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", transfer.calls, wantCalls)
	}
}
