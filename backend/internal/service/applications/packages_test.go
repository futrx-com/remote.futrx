package applications

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

// recordingCatalog stands in for the package store. The service's job is the
// policy around an upload — whether it is allowed, and what else has to happen
// when it succeeds — so what the store does with the bytes is irrelevant here.
type recordingCatalog struct {
	stored  []PackageView
	uploads []PackageUpload
	removed []string
	// pkg is what AddPackage reports having stored. The zero value stands
	// in for an ordinary UI package, so a test only sets it when the id or the
	// version is what it is about.
	pkg        Package
	installErr error
	removeErr  error
}

func (c *recordingCatalog) Packages() []PackageView { return c.stored }

func (c *recordingCatalog) AddPackage(upload PackageUpload) (PackageMutation, error) {
	c.uploads = append(c.uploads, upload)
	if c.installErr != nil {
		return PackageMutation{}, c.installErr
	}
	pkg := c.pkg
	if pkg.ID == "" {
		pkg = Package{ID: "uploaded-app", Name: "Uploaded App"}
	}
	replaced := false
	for _, existing := range c.stored {
		if existing.ID == pkg.ID {
			replaced = true
			break
		}
	}
	c.stored = append(c.stored, PackageView{Package: pkg})
	return PackageMutation{Package: pkg, Replaced: replaced}, nil
}

func (c *recordingCatalog) RemovePackage(id string) error {
	if c.removeErr != nil {
		return c.removeErr
	}
	c.removed = append(c.removed, id)
	return nil
}

func packageService(store *fakeStore, catalog PackageCatalog, host BackendHost) *Service {
	return New(
		&fakeRegistry{},
		store,
		nil,
		nil,
		nil,
		WithPackageCatalog(catalog),
		WithBackendHost(host),
	)
}

func TestUploadPackageStoresTheArchive(t *testing.T) {
	catalog := &recordingCatalog{}
	service := packageService(&fakeStore{}, catalog, nil)

	pkg, err := service.UploadPackage(context.Background(), PackageUpload{
		Filename: "app.zip",
		Data:     []byte("PK\x03\x04"),
		Actor:    "admin@example.com",
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if pkg.ID != "uploaded-app" {
		t.Fatalf("package = %+v", pkg)
	}
	if len(catalog.uploads) != 1 || catalog.uploads[0].Actor != "admin@example.com" {
		t.Fatalf("upload not forwarded intact: %+v", catalog.uploads)
	}
}

func TestUploadPackageRejectsAnEmptyArchive(t *testing.T) {
	service := packageService(&fakeStore{}, &recordingCatalog{}, nil)
	if _, err := service.UploadPackage(context.Background(), PackageUpload{}); !errors.Is(
		err, ErrPackageInvalid,
	) {
		t.Fatalf("err = %v, want %v", err, ErrPackageInvalid)
	}
}

// Replacing a package invalidates the application as one host operation. It
// must not depend on enumerating persisted instances: a process may be in the
// middle of launching, and a store read can fail after the catalog already
// committed the replacement.
func TestUploadPackageInvalidatesBackendsOfTheReplacedApplication(t *testing.T) {
	store := &fakeStore{
		global: []Instance{instance("uploaded-app", "", StatusRunning)},
		byProject: map[string][]Instance{
			"p1": {instance("uploaded-app", "p1", StatusRunning)},
			"p2": {instance("other-app", "p2", StatusRunning)},
		},
	}
	host := &recordingHost{}
	catalog := &recordingCatalog{stored: []PackageView{{Package: Package{
		ID: "uploaded-app", Name: "Uploaded App",
	}}}}
	service := packageService(store, catalog, host)

	if _, err := service.UploadPackage(context.Background(), PackageUpload{
		Data: []byte("PK\x03\x04"),
	}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(host.invalidated) != 1 || host.invalidated[0] != "uploaded-app" {
		t.Fatalf("invalidated = %v, want [uploaded-app]", host.invalidated)
	}
	if len(host.stopped) != 0 {
		t.Fatalf("package replacement stopped instances one by one: %v", host.stopped)
	}
}

func TestUploadPackageInvalidatesBeforeAListAllFailure(t *testing.T) {
	store := &fakeStore{listAllErr: errors.New("store unavailable")}
	host := &recordingHost{}
	catalog := &recordingCatalog{stored: []PackageView{{Package: Package{
		ID: "uploaded-app", Name: "Uploaded App",
	}}}}
	service := packageService(store, catalog, host)

	pkg, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK\x03\x04")})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if pkg.ID != "uploaded-app" {
		t.Fatalf("package = %+v", pkg)
	}
	if len(host.invalidated) != 1 || host.invalidated[0] != "uploaded-app" {
		t.Fatalf("invalidated = %v, want [uploaded-app]", host.invalidated)
	}
}

// Removing a package whose application is still installed would leave that
// instance pointing at a catalog entry that no longer exists.
func TestRemovePackageRefusesWhileTheApplicationIsInstalled(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		where string
		store *fakeStore
	}{
		{
			name:  "global",
			where: "globally",
			store: &fakeStore{global: []Instance{instance("uploaded-app", "", StatusRunning)}},
		},
		{
			name:  "project",
			where: "in project p1",
			store: &fakeStore{byProject: map[string][]Instance{
				"p1": {instance("uploaded-app", "p1", StatusStopped)},
			}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			catalog := &recordingCatalog{}
			service := packageService(testCase.store, catalog, nil)

			_, err := service.RemovePackage(
				context.Background(), RemovePackageRequest{ID: "uploaded-app"})
			if !errors.Is(err, ErrPackageInUse) {
				t.Fatalf("err = %v, want %v", err, ErrPackageInUse)
			}
			// The refusal names where it is installed, so the caller can act on
			// it instead of hunting through every project.
			if !strings.Contains(err.Error(), testCase.where) {
				t.Fatalf("error %q does not say where it is installed", err)
			}
			if len(catalog.removed) != 0 {
				t.Fatalf("the package was removed anyway: %v", catalog.removed)
			}
		})
	}
}

func TestRemovePackageDeletesItWhenNothingUsesIt(t *testing.T) {
	catalog := &recordingCatalog{}
	store := &fakeStore{global: []Instance{instance("other-app", "", StatusRunning)}}
	service := packageService(store, catalog, nil)

	if _, err := service.RemovePackage(
		context.Background(), RemovePackageRequest{ID: "uploaded-app"},
	); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(catalog.removed) != 1 || catalog.removed[0] != "uploaded-app" {
		t.Fatalf("removed = %v", catalog.removed)
	}
}

// A server built without a package store still serves its catalog; it just
// says so rather than pretending an upload worked.
func TestPackageRoutesReportUnavailableWithoutAStore(t *testing.T) {
	service := New(&fakeRegistry{}, &fakeStore{}, nil, nil, nil)
	ctx := context.Background()

	if _, err := service.Packages(ctx); !errors.Is(err, ErrPackagesUnavailable) {
		t.Fatalf("Packages err = %v", err)
	}
	if _, err := service.UploadPackage(ctx, PackageUpload{Data: []byte("x")}); !errors.Is(
		err, ErrPackagesUnavailable,
	) {
		t.Fatalf("UploadPackage err = %v", err)
	}
	if _, err := service.RemovePackage(ctx, RemovePackageRequest{ID: "x"}); !errors.Is(
		err, ErrPackagesUnavailable,
	) {
		t.Fatalf("RemovePackage err = %v", err)
	}
}

func TestPackagesListsWhatTheStoreHolds(t *testing.T) {
	catalog := &recordingCatalog{stored: []PackageView{{Package: Package{ID: "a"}}, {Package: Package{ID: "b"}, Error: "broken"}}}
	service := packageService(&fakeStore{}, catalog, nil)

	list, err := service.Packages(context.Background())
	if err != nil {
		t.Fatalf("packages: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Error != "" {
		t.Fatalf("a package that loaded must carry no error: %q", list[0].Error)
	}
	if list[1].Error == "" {
		t.Fatal("a package that failed to load must carry its reason")
	}
}

// The point of the cascade: "uninstall it everywhere first" is work the server
// can do, so asking for it is one request rather than a hunt through projects.
func TestRemovePackageUninstallsEveryCopyWhenAsked(t *testing.T) {
	store := &fakeStore{
		global: []Instance{instance("uploaded-app", "", StatusRunning)},
		byProject: map[string][]Instance{
			"p1": {instance("uploaded-app", "p1", StatusRunning)},
			"p2": {instance("other-app", "p2", StatusRunning)},
		},
	}
	catalog := &recordingCatalog{}
	installer := &recordingInstaller{}
	service := New(
		&fakeRegistry{withUI: []string{"uploaded-app", "other-app"}},
		store,
		installer,
		nil,
		nil,
		WithPackageCatalog(catalog),
	)

	uninstalled, err := service.RemovePackage(context.Background(), RemovePackageRequest{
		ID:                 "uploaded-app",
		UninstallInstalled: true,
	})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(uninstalled) != 2 {
		t.Fatalf("uninstalled = %+v, want 2", uninstalled)
	}
	deleted := map[string]bool{}
	for _, id := range store.deleted {
		deleted[id] = true
	}
	if !deleted["uploaded-app-"] || !deleted["uploaded-app-p1"] {
		t.Fatalf("copies of the package survived: %v", store.deleted)
	}
	if deleted["other-app-p2"] {
		t.Fatalf("an unrelated application was uninstalled: %v", store.deleted)
	}
	if len(catalog.removed) != 1 || catalog.removed[0] != "uploaded-app" {
		t.Fatalf("the package was not removed: %v", catalog.removed)
	}
}

// A package that no longer loads cannot describe how to tear its copies down —
// which is exactly when an operator most needs to be rid of it. Refusing here
// would leave them with a package they can neither repair nor remove.
func TestRemovePackageDropsCopiesOfAnUnloadableApplication(t *testing.T) {
	store := &fakeStore{global: []Instance{instance("broken-app", "", StatusError)}}
	catalog := &recordingCatalog{}
	host := &recordingHost{}
	// fakeRegistry knows nothing about "broken-app", so Uninstall reports the
	// application as unknown — the shape of a package whose files stopped loading.
	service := New(
		&fakeRegistry{}, store, nil, nil, nil,
		WithPackageCatalog(catalog), WithBackendHost(host),
	)

	uninstalled, err := service.RemovePackage(context.Background(), RemovePackageRequest{
		ID:                 "broken-app",
		UninstallInstalled: true,
	})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(uninstalled) != 1 {
		t.Fatalf("uninstalled = %+v, want 1", uninstalled)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "broken-app-" {
		t.Fatalf("the stranded record survived: %v", store.deleted)
	}
	// A backend is addressed by instance id, so it can be cleaned up without the
	// application. Leaving it would keep a process running as a child of the server
	// that no record points at any more.
	if len(host.removed) != 1 || host.removed[0] != "broken-app-" {
		t.Fatalf("the orphaned backend was left running: %v", host.removed)
	}
	if len(catalog.removed) != 1 {
		t.Fatalf("the package was not removed: %v", catalog.removed)
	}
}

// A cascade that cannot tear a copy down stops there and says which one. The
// package stays, so a retry after the container runtime is fixed does the
// whole job rather than half of it.
func TestRemovePackageReportsACopyItCouldNotUninstall(t *testing.T) {
	store := &fakeStore{global: []Instance{installedAt("g1", "", "1.0.0", StatusRunning)}}
	catalog := &recordingCatalog{}
	installer := &failingInstaller{}
	installer.uninstallErr = errors.New("lxd unavailable")
	service := New(
		&versionedRegistry{application: serviceApplicationAt("1.0.0")},
		store,
		installer,
		nil,
		nil,
		WithPackageCatalog(catalog),
	)

	uninstalled, err := service.RemovePackage(context.Background(), RemovePackageRequest{
		ID:                 "db",
		UninstallInstalled: true,
	})
	if !errors.Is(err, ErrPackageInUse) {
		t.Fatalf("err = %v, want %v", err, ErrPackageInUse)
	}
	if !strings.Contains(err.Error(), "lxd unavailable") {
		t.Fatalf("error %q does not carry the reason", err)
	}
	// The copy that stopped the removal is named by the error, not by a list of
	// removals that did not happen.
	if !strings.Contains(err.Error(), "globally") {
		t.Fatalf("error %q does not say which copy failed", err)
	}
	if uninstalled != nil {
		t.Fatalf("reported removals for a removal that failed: %+v", uninstalled)
	}
	if len(catalog.removed) != 0 {
		t.Fatalf("the package was removed despite a failed uninstall: %v", catalog.removed)
	}
}

// Listing packages says where each one is installed, which is what lets the
// caller decide about a removal before making it.
func TestPackagesReportWhereTheyAreInstalled(t *testing.T) {
	store := &fakeStore{
		global: []Instance{instance("uploaded-app", "", StatusRunning)},
		byProject: map[string][]Instance{
			"p1": {instance("uploaded-app", "p1", StatusStopped)},
		},
	}
	catalog := &recordingCatalog{stored: []PackageView{{Package: Package{ID: "uploaded-app"}}, {Package: Package{ID: "unused"}}}}
	service := packageService(store, catalog, nil)

	list, err := service.Packages(context.Background())
	if err != nil {
		t.Fatalf("packages: %v", err)
	}
	if len(list[0].Installs) != 2 {
		t.Fatalf("installs = %+v, want 2", list[0].Installs)
	}
	if len(list[1].Installs) != 0 {
		t.Fatalf("an uninstalled package reported installs: %+v", list[1].Installs)
	}
	scopes := map[Scope]string{}
	for _, install := range list[0].Installs {
		scopes[install.Scope] = install.ProjectID
	}
	if _, ok := scopes[ScopeGlobal]; !ok {
		t.Fatalf("the global copy is missing: %+v", list[0].Installs)
	}
	if scopes[ScopeProject] != "p1" {
		t.Fatalf("the project copy is missing its project: %+v", list[0].Installs)
	}
}

// A release that builds in an application people had been uploading leaves
// their package on disk, shadowed by the application that replaced it. What is
// installed under that id belongs to the built-in application from then on, so the
// stored files are inert — and deleting inert files must not offer, let alone
// agree, to take down the applications still running under that name.
func TestRemovingASupersededPackageLeavesItsInstallsAlone(t *testing.T) {
	catalog := &recordingCatalog{stored: []PackageView{{Package: Package{ID: "s3disk"}, Error: "superseded"}}}
	store := &fakeStore{global: []Instance{instance("s3disk", "", StatusRunning)}}
	service := New(
		&fakeRegistry{builtin: []string{"s3disk"}}, store, nil, nil, nil,
		WithPackageCatalog(catalog),
	)
	ctx := context.Background()

	// The list is what the UI decides from: a row carrying installs is a row
	// whose remove button turns into "uninstall and remove".
	list, err := service.Packages(ctx)
	if err != nil {
		t.Fatalf("packages: %v", err)
	}
	if len(list) != 1 || len(list[0].Installs) != 0 {
		t.Fatalf("superseded package claimed the built-in application's copies: %+v", list)
	}

	// Removing it is not refused, though a copy is installed under its id.
	if _, err := service.RemovePackage(ctx, RemovePackageRequest{ID: "s3disk"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(catalog.removed) != 1 || catalog.removed[0] != "s3disk" {
		t.Fatalf("removed = %v", catalog.removed)
	}
	// And asking for the cascade does not get one either. There is nothing
	// here the cascade exists to protect against.
	if _, err := service.RemovePackage(ctx, RemovePackageRequest{
		ID: "s3disk", UninstallInstalled: true,
	}); err != nil {
		t.Fatalf("remove with cascade: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("a working application was uninstalled to delete a dead package: %v", store.deleted)
	}
}

// TestPackageViewIsOneFlatObject pins the wire shape that splitting the stored
// record from the reported view has to leave alone: the SPA reads one object
// with every key at the top level, and the record written to the metadata file
// carries none of the three fields that are only true while the server runs.
func TestPackageViewIsOneFlatObject(t *testing.T) {
	view := PackageView{
		Package: Package{
			ID:         "s3disk",
			Name:       "S3 Disk",
			Version:    "1.2.0",
			Scopes:     []Scope{ScopeGlobal, ScopeProject},
			Filename:   "s3disk.zip",
			Size:       2048,
			SHA256:     "9f86d08",
			UploadedAt: 1700000000,
			UploadedBy: "admin@example.com",
		},
		Installs: []PackageInstall{{
			InstanceID: "inst-1", Name: "S3", Scope: ScopeGlobal, Status: StatusRunning,
		}},
		Upgraded: []UpgradeOutcome{{InstanceID: "inst-1", To: "1.2.0"}},
		Error:    "shadowed by a built-in",
	}

	reported := marshalKeys(t, view)
	for _, key := range []string{
		"id", "name", "version", "scopes", "filename", "size", "sha256",
		"uploadedAt", "uploadedBy", "installs", "upgraded", "error",
	} {
		if _, ok := reported[key]; !ok {
			t.Errorf("the API no longer reports %q: %v", key, sortedKeys(reported))
		}
	}
	if _, nested := reported["Package"]; nested {
		t.Fatalf("the embedded record surfaced as its own object: %v", sortedKeys(reported))
	}

	onDisk := marshalKeys(t, view.Package)
	for _, key := range []string{"installs", "upgraded", "error"} {
		if _, ok := onDisk[key]; ok {
			t.Errorf("%q is written to the package metadata file: %v", key, sortedKeys(onDisk))
		}
	}
}

func marshalKeys(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keyed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keyed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return keyed
}

func sortedKeys(keyed map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(keyed))
	for key := range keyed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
