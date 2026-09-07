package applications

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// zipOf builds an archive from a name -> content map.
func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range files {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buffer.Bytes()
}

const uploadedManifest = `{
	"id": "uploaded-app",
	"name": "Uploaded App",
	"version": "1.0.0",
	"type": "ui",
	"scopes": ["global", "project"]
}`

func uploadedPackage() map[string]string {
	return map[string]string{
		"image.json":          uploadedManifest,
		"ui/scripts/main.js":  "export default () => {}\n",
		"ui/style/app.css":    ".app{}\n",
		"ui/views/panel.html": "<p>panel</p>\n",
	}
}

// newTestRegistry builds a registry over the fixture catalog and an empty
// package store rooted in a temporary directory, returning both.
func newTestRegistry(t *testing.T) (*Registry, *PackageStore, string) {
	t.Helper()
	root := t.TempDir()
	store, err := NewPackageStore(root)
	if err != nil {
		t.Fatalf("new package store: %v", err)
	}
	registry, err := NewRegistryWithPackages(fixtureCatalog(), store)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	return registry, store, root
}

func upload(t *testing.T, r *Registry, files map[string]string) svc.Package {
	t.Helper()
	pkg, err := r.InstallPackage(svc.PackageUpload{
		Filename: "app.zip",
		Data:     zipOf(t, files),
		Actor:    "admin@example.com",
	})
	if err != nil {
		t.Fatalf("install package: %v", err)
	}
	return pkg
}

func TestInstallPackageJoinsTheCatalog(t *testing.T) {
	registry, _, _ := newTestRegistry(t)

	pkg := upload(t, registry, uploadedPackage())
	if pkg.ID != "uploaded-app" || pkg.Name != "Uploaded App" || pkg.Version != "1.0.0" {
		t.Fatalf("unexpected package metadata: %+v", pkg)
	}
	if pkg.UploadedBy != "admin@example.com" || pkg.UploadedAt == 0 || pkg.Size == 0 {
		t.Fatalf("upload provenance not recorded: %+v", pkg)
	}

	img, ok := registry.Get("uploaded-app")
	if !ok {
		t.Fatal("uploaded image is not in the catalog")
	}
	if img.Source != svc.SourceUploaded {
		t.Fatalf("source = %q, want %q", img.Source, svc.SourceUploaded)
	}
	if img.UI == nil || img.UI.Entry != "scripts/main.js" {
		t.Fatalf("ui not discovered: %+v", img.UI)
	}
	// The catalog stays sorted and still holds everything it shipped with.
	found := false
	for _, entry := range registry.List() {
		if entry.ID == "uploaded-app" {
			found = true
		}
	}
	if !found {
		t.Fatal("List() omits the uploaded image")
	}
	if _, ok := registry.Get(fixtureService); !ok {
		t.Fatal("built-in image disappeared after an upload")
	}
}

func TestUploadedImageServesItsOwnAssets(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	upload(t, registry, uploadedPackage())

	asset, ok := registry.UIAsset("uploaded-app", "scripts/main.js")
	if !ok || !strings.Contains(string(asset), "export default") {
		t.Fatalf("UIAsset = %q, %v", asset, ok)
	}
	// Traversal out of the uploaded image's ui/ is refused exactly as it is
	// for a built-in one.
	if _, ok := registry.UIAsset("uploaded-app", "../image.json"); ok {
		t.Fatal("UIAsset escaped the image's ui/ directory")
	}
}

func TestUploadedPluginSourceComesFromThePackage(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	upload(t, registry, map[string]string{
		"image.json": `{
			"id": "uploaded-backend",
			"name": "Uploaded Backend",
			"version": "1.0.0",
			"type": "backend",
			"scopes": ["global"]
		}`,
		"plugin/main.go": "package main\n\nfunc main() {}\n",
	})

	source, ok := registry.PluginSource("uploaded-backend")
	if !ok {
		t.Fatal("PluginSource not available for the uploaded image")
	}
	data, err := fs.ReadFile(source, "main.go")
	if err != nil || !strings.Contains(string(data), "package main") {
		t.Fatalf("plugin source = %q, %v", data, err)
	}
}

func TestUploadedServiceCarriesItsInstallScript(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	upload(t, registry, map[string]string{
		"image.json": `{
			"id": "uploaded-service",
			"name": "Uploaded Service",
			"version": "1.0.0",
			"scopes": ["global"],
			"port": {"internal": 6000},
			"service": "uploaded"
		}`,
		"install.sh": "#!/usr/bin/env bash\necho uploaded\n",
	})

	script, ok := registry.Script("uploaded-service")
	if !ok || !strings.Contains(string(script), "echo uploaded") {
		t.Fatalf("Script = %q, %v", script, ok)
	}
}

func TestPackageArchiveMayWrapItsFilesInOneFolder(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	// What a desktop "compress this folder" produces, plus the bookkeeping
	// files macOS adds to the archive.
	upload(t, registry, map[string]string{
		"my-app/image.json":            uploadedManifest,
		"my-app/ui/scripts/main.js":    "export default () => {}\n",
		"my-app/.DS_Store":             "junk",
		"__MACOSX/my-app/._image.json": "junk",
	})

	if _, ok := registry.Get("uploaded-app"); !ok {
		t.Fatal("wrapped archive did not install")
	}
}

func TestPackageIsRejectedWhenItCannotBecomeAnImage(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  error
	}{
		{
			name:  "no manifest",
			files: map[string]string{"ui/scripts/main.js": "x"},
			want:  svc.ErrPackageInvalid,
		},
		{
			name:  "manifest is not json",
			files: map[string]string{"image.json": "{"},
			want:  svc.ErrPackageInvalid,
		},
		{
			name:  "manifest declares no id",
			files: map[string]string{"image.json": `{"name": "X", "scopes": ["global"]}`},
			want:  svc.ErrPackageInvalid,
		},
		{
			name:  "id is not a safe directory name",
			files: map[string]string{"image.json": `{"id": "../escape", "name": "X", "scopes": ["global"]}`},
			want:  svc.ErrPackageInvalid,
		},
		{
			name: "escaping member path",
			files: map[string]string{
				"image.json":     uploadedManifest,
				"../outside.txt": "x",
			},
			want: svc.ErrPackageInvalid,
		},
		{
			name: "image.json fails catalog validation",
			files: map[string]string{
				"image.json": `{"id": "broken", "name": "Broken", "scopes": ["nowhere"]}`,
			},
			want: svc.ErrPackageInvalid,
		},
		{
			name: "ui type ships no ui directory",
			files: map[string]string{
				"image.json": `{"id": "hollow", "name": "Hollow", "type": "ui", "scopes": ["global"]}`,
			},
			want: svc.ErrPackageInvalid,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry, _, root := newTestRegistry(t)
			_, err := registry.InstallPackage(svc.PackageUpload{Data: zipOf(t, testCase.files)})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
			// A refused upload writes nothing into the committed catalog.
			entries, _ := os.ReadDir(filepath.Join(root, packageImagesDir))
			if len(entries) != 0 {
				t.Fatalf("a rejected upload left %d directories behind", len(entries))
			}
		})
	}
}

func TestPackageCannotShadowABuiltInImage(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	_, err := registry.InstallPackage(svc.PackageUpload{Data: zipOf(t, map[string]string{
		"image.json": `{
			"id": "` + fixtureUI + `",
			"name": "Impostor",
			"type": "ui",
			"scopes": ["global"]
		}`,
		"ui/scripts/main.js": "export default () => {}\n",
	})})
	if !errors.Is(err, svc.ErrPackageReserved) {
		t.Fatalf("err = %v, want %v", err, svc.ErrPackageReserved)
	}
	if img, _ := registry.Get(fixtureUI); img.Source != svc.SourceBuiltin {
		t.Fatalf("built-in image was replaced: %+v", img)
	}
}

func TestUploadingAgainReplacesTheStoredPackage(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	upload(t, registry, uploadedPackage())

	next := uploadedPackage()
	next["image.json"] = strings.Replace(uploadedManifest, `"1.0.0"`, `"2.0.0"`, 1)
	next["ui/scripts/main.js"] = "export default () => 2;\n"
	delete(next, "ui/views/panel.html")
	upload(t, registry, next)

	img, _ := registry.Get("uploaded-app")
	if img.Version != "2.0.0" {
		t.Fatalf("version = %q, want 2.0.0", img.Version)
	}
	if _, ok := img.UI.Views["panel"]; ok {
		t.Fatal("a view the new package dropped is still in the catalog")
	}
	asset, _ := registry.UIAsset("uploaded-app", "scripts/main.js")
	if !strings.Contains(string(asset), "() => 2") {
		t.Fatalf("stale asset served after replacement: %q", asset)
	}
	if packages := registry.Packages(); len(packages) != 1 {
		t.Fatalf("replacing produced %d packages, want 1", len(packages))
	}
}

func TestFailedReplacementLeavesThePreviousPackageInstalled(t *testing.T) {
	registry, _, _ := newTestRegistry(t)
	upload(t, registry, uploadedPackage())

	_, err := registry.InstallPackage(svc.PackageUpload{Data: zipOf(t, map[string]string{
		"image.json": `{"id": "uploaded-app", "name": "Broken", "type": "ui", "scopes": ["global"]}`,
	})})
	if !errors.Is(err, svc.ErrPackageInvalid) {
		t.Fatalf("err = %v, want %v", err, svc.ErrPackageInvalid)
	}
	img, ok := registry.Get("uploaded-app")
	if !ok || img.Version != "1.0.0" || img.UI == nil {
		t.Fatalf("the working package did not survive a failed replacement: %+v", img)
	}
}

func TestRemovePackageDropsItFromTheCatalog(t *testing.T) {
	registry, _, root := newTestRegistry(t)
	upload(t, registry, uploadedPackage())

	if err := registry.RemovePackage("uploaded-app"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := registry.Get("uploaded-app"); ok {
		t.Fatal("removed image is still in the catalog")
	}
	if len(registry.Packages()) != 0 {
		t.Fatal("removed package is still listed")
	}
	if _, err := os.Stat(filepath.Join(root, packageImagesDir, "uploaded-app")); !os.IsNotExist(err) {
		t.Fatalf("package files survived removal: %v", err)
	}
	if err := registry.RemovePackage("uploaded-app"); !errors.Is(err, svc.ErrPackageNotFound) {
		t.Fatalf("second remove err = %v, want %v", err, svc.ErrPackageNotFound)
	}
	if err := registry.RemovePackage(fixtureUI); !errors.Is(err, svc.ErrPackageReserved) {
		t.Fatalf("removing a built-in err = %v, want %v", err, svc.ErrPackageReserved)
	}
}

// A package is state, not program: a new binary reading the same directory has
// to find every uploaded application still there. This is the property that
// makes updating Remote safe for them.
func TestUploadedPackagesSurviveARestart(t *testing.T) {
	registry, _, root := newTestRegistry(t)
	upload(t, registry, uploadedPackage())

	reopened, err := NewPackageStore(root)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	restarted, err := NewRegistryWithPackages(fixtureCatalog(), reopened)
	if err != nil {
		t.Fatalf("restart registry: %v", err)
	}
	img, ok := restarted.Get("uploaded-app")
	if !ok || img.Source != svc.SourceUploaded {
		t.Fatalf("uploaded image did not survive a restart: %+v", img)
	}
	packages := restarted.Packages()
	if len(packages) != 1 || packages[0].UploadedBy != "admin@example.com" {
		t.Fatalf("package provenance did not survive a restart: %+v", packages)
	}
}

// A package written against a different version of the server must not be able
// to stop the server from starting: it is skipped, and the reason is reported.
func TestUnloadablePackageIsReportedRatherThanFatal(t *testing.T) {
	registry, _, root := newTestRegistry(t)
	upload(t, registry, uploadedPackage())
	if err := os.WriteFile(
		filepath.Join(root, packageImagesDir, "uploaded-app", "image.json"),
		[]byte(`{"id": "uploaded-app", "name": "X", "version": "1.0.0", "scopes": ["from-the-future"]}`),
		0o600,
	); err != nil {
		t.Fatalf("corrupt package: %v", err)
	}

	restarted, err := NewRegistryWithPackages(fixtureCatalog(), mustStore(t, root))
	if err != nil {
		t.Fatalf("a broken package must not fail the load: %v", err)
	}
	if _, ok := restarted.Get("uploaded-app"); ok {
		t.Fatal("a package that does not load must not appear in the catalog")
	}
	if _, ok := restarted.Get(fixtureService); !ok {
		t.Fatal("a broken package took the built-in catalog down with it")
	}
	packages := restarted.Packages()
	if len(packages) != 1 || packages[0].Installed() {
		t.Fatalf("broken package not reported: %+v", packages)
	}
	if !strings.Contains(packages[0].Error, "from-the-future") {
		t.Fatalf("reported reason does not say what is wrong: %q", packages[0].Error)
	}
}

func mustStore(t *testing.T, root string) *PackageStore {
	t.Helper()
	store, err := NewPackageStore(root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

func TestRegistryWithoutAPackageStoreRefusesUploads(t *testing.T) {
	registry, err := NewRegistryFromFS(fixtureCatalog())
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if _, err := registry.InstallPackage(svc.PackageUpload{Data: []byte("x")}); !errors.Is(
		err, svc.ErrPackagesUnavailable,
	) {
		t.Fatalf("err = %v, want %v", err, svc.ErrPackagesUnavailable)
	}
	if registry.Packages() != nil {
		t.Fatal("a registry with no store must list no packages")
	}
	for _, img := range registry.List() {
		if img.Source != svc.SourceBuiltin {
			t.Fatalf("%s: source = %q, want builtin", img.ID, img.Source)
		}
	}
}

func TestPackageFilesAreNotWrittenExecutable(t *testing.T) {
	registry, _, root := newTestRegistry(t)
	upload(t, registry, map[string]string{
		"image.json": `{
			"id": "uploaded-service",
			"name": "Uploaded Service",
			"version": "1.0.0",
			"scopes": ["global"],
			"port": {"internal": 6000},
			"service": "uploaded"
		}`,
		"install.sh": "#!/usr/bin/env bash\n",
	})
	info, err := os.Stat(filepath.Join(root, packageImagesDir, "uploaded-service", "install.sh"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("mode = %v, want no execute bits", info.Mode().Perm())
	}
}

// The other side of "a package may not shadow a built-in image": a package
// stored before its id was built in. Its files are still on disk, the catalog
// serves the built-in image instead, and deleting them is the only thing left
// to do with them — so the refusal that stops a DELETE from reaching "mysql"
// must not also apply to the one package that has to be removable.
func TestPackageSupersededByABuiltInImageCanStillBeRemoved(t *testing.T) {
	root := t.TempDir()
	store := mustStore(t, root)

	// Uploaded to a server whose binary defined no such image.
	before, err := NewRegistryWithPackages(fixtureCatalog(), store)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	upload(t, before, uploadedPackage())

	// The release that built that application in: the same files on disk, a
	// catalog that now defines the same id.
	catalog := fixtureCatalog()
	catalog["images/uploaded-app/image.json"] = &fstest.MapFile{Data: []byte(`{
		"name": "Uploaded App",
		"version": "2.0.0",
		"type": "ui",
		"scopes": ["global", "project"]
	}`)}
	catalog["images/uploaded-app/ui/scripts/main.js"] = &fstest.MapFile{
		Data: []byte("export default () => {}\n"),
	}
	after, err := NewRegistryWithPackages(catalog, store)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	img, ok := after.Get("uploaded-app")
	if !ok || img.Source != svc.SourceBuiltin || img.Version != "2.0.0" {
		t.Fatalf("the built-in image is not what the catalog serves: %+v", img)
	}
	stored := after.Packages()
	if len(stored) != 1 || !strings.Contains(stored[0].Error, svc.ErrPackageSuperseded.Error()) {
		t.Fatalf("the stored package is not reported as superseded: %+v", stored)
	}

	if err := after.RemovePackage("uploaded-app"); err != nil {
		t.Fatalf("remove superseded package: %v", err)
	}
	if remaining := after.Packages(); len(remaining) != 0 {
		t.Fatalf("the files survived the removal: %+v", remaining)
	}
	if img, ok := after.Get("uploaded-app"); !ok || img.Source != svc.SourceBuiltin {
		t.Fatalf("removing the shadowed package took the built-in image with it: %+v", img)
	}

	// An id that is only built in, with nothing stored under it, is still
	// refused rather than reported missing.
	if err := after.RemovePackage(fixtureUI); !errors.Is(err, svc.ErrPackageReserved) {
		t.Fatalf("RemovePackage(%q) err = %v, want %v", fixtureUI, err, svc.ErrPackageReserved)
	}
}
