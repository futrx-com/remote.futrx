package applications

import (
	"context"
	"errors"
	"testing"
)

// versionedRegistry answers with one image, so a test can move its version and
// nothing else.
type versionedRegistry struct{ image Image }

func (r *versionedRegistry) List() []Image { return []Image{r.image} }

func (r *versionedRegistry) Get(id string) (Image, bool) {
	if id != r.image.ID {
		return Image{}, false
	}
	return r.image, true
}

func (r *versionedRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

// failingInstaller records installs and can be made to fail, so the error path
// of an upgrade is exercised against the same recorder as the success path.
type failingInstaller struct {
	recordingInstaller
	installErr   error
	uninstallErr error
}

func (i *failingInstaller) Uninstall(ctx context.Context, spec InstallSpec) error {
	if i.uninstallErr != nil {
		return i.uninstallErr
	}
	return i.recordingInstaller.Uninstall(ctx, spec)
}

func (i *failingInstaller) Install(ctx context.Context, spec InstallSpec) error {
	if i.installErr != nil {
		return i.installErr
	}
	return i.recordingInstaller.Install(ctx, spec)
}

func serviceImageAt(version string) Image {
	return Image{
		ID:      "db",
		Name:    "Database",
		Version: version,
		Type:    KindService,
		Scopes:  []Scope{ScopeGlobal, ScopeProject},
		Port:    Port{Internal: 5432},
		Service: "db",
	}
}

// installedAt builds an instance of "db" recorded against a given version.
func installedAt(id, projectID, version string, status InstanceStatus) Instance {
	scope := ScopeGlobal
	container := "futrx-app-" + id
	if projectID != "" {
		scope = ScopeProject
		container = "project-" + projectID
	}
	return Instance{
		ID:            id,
		ImageID:       "db",
		ImageVersion:  version,
		Name:          "Database",
		Scope:         scope,
		ProjectID:     projectID,
		ContainerName: container,
		InternalPort:  5432,
		ExternalPort:  5433,
		Status:        status,
	}
}

func upgradeService(
	store *fakeStore,
	registry *versionedRegistry,
	installer Installer,
	catalog PackageCatalog,
	host BackendHost,
) *Service {
	return New(
		registry,
		store,
		installer,
		&staticProjects{container: "project-container"},
		&countingAllocator{},
		WithPackageCatalog(catalog),
		WithBackendHost(host),
	)
}

// The whole point of the version field: a package whose version moved
// re-provisions what it already installed.
func TestUploadReinstallsInstancesWhenTheVersionChanges(t *testing.T) {
	store := &fakeStore{
		global: []Instance{installedAt("g1", "", "1.0.0", StatusRunning)},
		byProject: map[string][]Instance{
			"p1": {installedAt("p1i", "p1", "1.0.0", StatusRunning)},
		},
	}
	installer := &failingInstaller{}
	registry := &versionedRegistry{image: serviceImageAt("2.0.0")}
	catalog := &recordingCatalog{pkg: Package{ID: "db", Name: "Database", Version: "2.0.0"}}
	service := upgradeService(store, registry, installer, catalog, &recordingHost{})

	pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(installer.installed) != 2 {
		t.Fatalf("install script ran %d times, want 2", len(installer.installed))
	}
	// The re-install reuses the container and port the instance already had:
	// an upgrade replaces software, not the address clients connect to.
	for _, spec := range installer.installed {
		if spec.Instance.ContainerName == "" || spec.Instance.ExternalPort != 5433 {
			t.Fatalf("upgrade moved the instance: %+v", spec.Instance)
		}
		if spec.Image.Version != "2.0.0" {
			t.Fatalf("re-installed with version %q, want 2.0.0", spec.Image.Version)
		}
	}
	if len(pkg.Upgraded) != 2 {
		t.Fatalf("upgrade outcomes = %+v, want 2", pkg.Upgraded)
	}
	for _, outcome := range pkg.Upgraded {
		if outcome.From != "1.0.0" || outcome.To != "2.0.0" || outcome.Error != "" {
			t.Fatalf("outcome = %+v", outcome)
		}
	}
	// The new version is recorded, so a second upload of the same version is
	// not a second re-install.
	for _, inst := range store.puts {
		if inst.ImageVersion != "2.0.0" {
			t.Fatalf("%s recorded version %q, want 2.0.0", inst.ID, inst.ImageVersion)
		}
	}
}

// Re-uploading without touching the version is how an author says "nothing
// that lives in a container changed". It must not disturb a running database.
func TestUploadDoesNotReinstallWhenTheVersionIsUnchanged(t *testing.T) {
	store := &fakeStore{global: []Instance{installedAt("g1", "", "1.0.0", StatusRunning)}}
	installer := &failingInstaller{}
	registry := &versionedRegistry{image: serviceImageAt("1.0.0")}
	catalog := &recordingCatalog{pkg: Package{ID: "db", Version: "1.0.0"}}
	host := &recordingHost{}
	service := upgradeService(store, registry, installer, catalog, host)

	pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(installer.installed) != 0 {
		t.Fatalf("install script ran %d times for an unchanged version", len(installer.installed))
	}
	if len(pkg.Upgraded) != 0 {
		t.Fatalf("upgrade outcomes = %+v, want none", pkg.Upgraded)
	}
	// The plugin is still refreshed: its source may have changed even when the
	// container side did not, and restarting it costs nothing.
	if len(host.stopped) != 1 || host.stopped[0] != "g1" {
		t.Fatalf("plugin was not refreshed: %v", host.stopped)
	}
}

// An instance stored before versions were tracked has no recorded version.
// Install scripts are idempotent by contract, so re-running is the safe answer
// to "we do not know what is in there".
func TestUploadReinstallsAnInstanceWithNoRecordedVersion(t *testing.T) {
	store := &fakeStore{global: []Instance{installedAt("g1", "", "", StatusRunning)}}
	installer := &failingInstaller{}
	registry := &versionedRegistry{image: serviceImageAt("1.0.0")}
	service := upgradeService(
		store, registry, installer, &recordingCatalog{pkg: Package{ID: "db"}}, nil)

	pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(installer.installed) != 1 {
		t.Fatalf("install script ran %d times, want 1", len(installer.installed))
	}
	if len(pkg.Upgraded) != 1 || pkg.Upgraded[0].From != "" || pkg.Upgraded[0].To != "1.0.0" {
		t.Fatalf("outcomes = %+v", pkg.Upgraded)
	}
}

// A ui or backend image provisions nothing into a container, so there is no
// install script for a version bump to re-run.
func TestUploadDoesNotReinstallImagesWithNoContainerSide(t *testing.T) {
	for _, kind := range []Kind{KindUI, KindBackend} {
		t.Run(string(kind), func(t *testing.T) {
			image := serviceImageAt("2.0.0")
			image.Type = kind
			image.Port = Port{}
			image.Service = ""
			store := &fakeStore{global: []Instance{installedAt("g1", "", "1.0.0", StatusRunning)}}
			installer := &failingInstaller{}
			service := upgradeService(
				store,
				&versionedRegistry{image: image},
				installer,
				&recordingCatalog{pkg: Package{ID: "db", Version: "2.0.0"}},
				&recordingHost{},
			)

			pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")})
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if len(installer.installed) != 0 {
				t.Fatalf("%s image ran an install script", kind)
			}
			if len(pkg.Upgraded) != 0 {
				t.Fatalf("outcomes = %+v, want none", pkg.Upgraded)
			}
		})
	}
}

// Re-running an install script also brings the app up. An upload must not
// resurrect an app someone deliberately stopped.
func TestUploadLeavesStoppedInstancesAlone(t *testing.T) {
	store := &fakeStore{global: []Instance{installedAt("g1", "", "1.0.0", StatusStopped)}}
	installer := &failingInstaller{}
	service := upgradeService(
		store,
		&versionedRegistry{image: serviceImageAt("2.0.0")},
		installer,
		&recordingCatalog{pkg: Package{ID: "db", Version: "2.0.0"}},
		nil,
	)

	pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(installer.installed) != 0 {
		t.Fatal("a stopped instance was restarted by an upload")
	}
	if len(pkg.Upgraded) != 0 {
		t.Fatalf("outcomes = %+v, want none", pkg.Upgraded)
	}
}

// …and the upgrade it skipped happens when its owner starts it again.
func TestStartingAStaleInstanceReinstallsIt(t *testing.T) {
	store := &fakeStore{global: []Instance{installedAt("g1", "", "1.0.0", StatusStopped)}}
	installer := &failingInstaller{}
	service := upgradeService(
		store, &versionedRegistry{image: serviceImageAt("2.0.0")}, installer, nil, nil)

	view, err := service.Start(context.Background(), "g1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(installer.installed) != 1 {
		t.Fatalf("starting a stale instance ran %d installs, want 1", len(installer.installed))
	}
	if view.ImageVersion != "2.0.0" {
		t.Fatalf("recorded version = %q, want 2.0.0", view.ImageVersion)
	}

	// Starting a current instance is an ordinary start, not a re-install.
	installer.installed = nil
	if _, err := service.Start(context.Background(), "g1"); err != nil {
		t.Fatalf("second start: %v", err)
	}
	if len(installer.installed) != 1 {
		t.Fatalf("a current instance was re-installed rather than started: %d", len(installer.installed))
	}
}

// A failed upgrade must be visible and must not claim the new version, or the
// next upload would skip the instance as already current.
func TestFailedUpgradeIsReportedAndKeepsTheOldVersion(t *testing.T) {
	store := &fakeStore{global: []Instance{installedAt("g1", "", "1.0.0", StatusRunning)}}
	installer := &failingInstaller{installErr: errors.New("apt-get failed")}
	service := upgradeService(
		store,
		&versionedRegistry{image: serviceImageAt("2.0.0")},
		installer,
		&recordingCatalog{pkg: Package{ID: "db", Version: "2.0.0"}},
		nil,
	)

	// The upload itself still succeeds: the package is stored, and the failure
	// belongs to one instance.
	pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(pkg.Upgraded) != 1 || pkg.Upgraded[0].Error == "" {
		t.Fatalf("outcomes = %+v, want one failure", pkg.Upgraded)
	}
	last := store.puts[len(store.puts)-1]
	if last.Status != StatusError {
		t.Fatalf("status = %q, want %q", last.Status, StatusError)
	}
	if last.ImageVersion != "1.0.0" {
		t.Fatalf("a failed upgrade recorded version %q; it must keep 1.0.0", last.ImageVersion)
	}
}

// A fresh install records the version it came from, which is what every
// upgrade decision later reads.
func TestInstallRecordsTheImageVersion(t *testing.T) {
	service := upgradeService(
		&fakeStore{}, &versionedRegistry{image: serviceImageAt("3.1.4")}, &failingInstaller{}, nil, nil)

	view, err := service.Install(context.Background(), InstallRequest{
		ImageID: "db",
		Scope:   ScopeGlobal,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if view.ImageVersion != "3.1.4" {
		t.Fatalf("recorded version = %q, want 3.1.4", view.ImageVersion)
	}
}

// An image installs only where it says it does. The catalog grid hides a card
// it cannot install and the API refuses the request anyway, because a UI that
// filters is a convenience and the service is the rule.
func TestInstallRefusesAScopeTheImageDoesNotDeclare(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		scopes  []Scope
		kind    Kind
		request InstallRequest
	}{
		{
			name:    "project-only service installed globally",
			scopes:  []Scope{ScopeProject},
			kind:    KindService,
			request: InstallRequest{ImageID: "db", Scope: ScopeGlobal},
		},
		{
			name:    "project-only ui installed globally",
			scopes:  []Scope{ScopeProject},
			kind:    KindUI,
			request: InstallRequest{ImageID: "db", Scope: ScopeGlobal},
		},
		{
			name:    "project-only backend installed globally",
			scopes:  []Scope{ScopeProject},
			kind:    KindBackend,
			request: InstallRequest{ImageID: "db", Scope: ScopeGlobal},
		},
		{
			name:    "global-only service installed into a project",
			scopes:  []Scope{ScopeGlobal},
			kind:    KindService,
			request: InstallRequest{ImageID: "db", Scope: ScopeProject, ProjectID: "p1"},
		},
		{
			name:    "unknown scope",
			scopes:  []Scope{ScopeGlobal, ScopeProject},
			kind:    KindService,
			request: InstallRequest{ImageID: "db", Scope: "host"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			image := serviceImageAt("1.0.0")
			image.Scopes = testCase.scopes
			image.Type = testCase.kind
			if !testCase.kind.NeedsPort() {
				image.Port = Port{}
				image.Service = ""
			}
			store := &fakeStore{}
			installer := &failingInstaller{}
			service := upgradeService(store, &versionedRegistry{image: image}, installer, nil, nil)

			if _, err := service.Install(context.Background(), testCase.request); !errors.Is(
				err, ErrScope,
			) {
				t.Fatalf("err = %v, want %v", err, ErrScope)
			}
			// Nothing was recorded and nothing was provisioned: a refused scope
			// must not leave a half-installed instance behind.
			if len(store.puts) != 0 {
				t.Fatalf("a refused install persisted %d instances", len(store.puts))
			}
			if len(installer.installed) != 0 {
				t.Fatalf("a refused install ran %d install scripts", len(installer.installed))
			}
		})
	}
}

// The scopes an uploaded package declares are what the catalog reports, so the
// management list can say where it installs rather than implying "everywhere".
func TestInstallAcceptsEveryScopeTheImageDeclares(t *testing.T) {
	image := serviceImageAt("1.0.0")
	image.Scopes = []Scope{ScopeProject}
	service := upgradeService(
		&fakeStore{}, &versionedRegistry{image: image}, &failingInstaller{}, nil, nil)

	if _, err := service.Install(context.Background(), InstallRequest{
		ImageID:   "db",
		Scope:     ScopeProject,
		ProjectID: "p1",
	}); err != nil {
		t.Fatalf("project install of a project-scoped image: %v", err)
	}
}
