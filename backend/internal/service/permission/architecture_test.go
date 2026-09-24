package permission

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/futrx-com/remote.futrx.com/internal/"

// backendRoot is the repository's backend directory as seen from this
// package's directory, where `go test` runs.
const backendRoot = "../../.."

type sourceFile struct {
	path    string // slash-separated, relative to the backend directory
	dir     string
	file    *ast.File
	imports []string // import paths relative to internal/, for module imports
}

// productionSources parses every non-test Go file under the backend.
func productionSources(t *testing.T) []sourceFile {
	t.Helper()
	var files []sourceFile
	err := filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "node_modules" || name == "vendors" || name == "public" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(backendRoot, path)
		source := sourceFile{path: filepath.ToSlash(relative), dir: filepath.ToSlash(filepath.Dir(relative)), file: parsed}
		for _, spec := range parsed.Imports {
			importPath, _ := strconv.Unquote(spec.Path.Value)
			if rest, ok := strings.CutPrefix(importPath, modulePath); ok {
				source.imports = append(source.imports, rest)
			}
		}
		files = append(files, source)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func importsAny(imports []string, forbidden ...string) (string, bool) {
	for _, imported := range imports {
		for _, prefix := range forbidden {
			if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
				return imported, true
			}
		}
	}
	return "", false
}

// The permission domain must stay independent of the things it consults
// through ports: identity, membership, and persistence.
func TestPermissionPackageDependsOnlyOnItsOwnPorts(t *testing.T) {
	forbidden := []string{
		"service/auth", "service/user", "service/project", "service/chat",
		"stores", "transport",
	}
	for _, source := range productionSources(t) {
		if source.dir != "internal/service/permission" {
			continue
		}
		if imported, bad := importsAny(source.imports, forbidden...); bad {
			t.Errorf("%s imports %s; the permission domain must use its ports instead", source.path, imported)
		}
	}
}

// A service that enforces a permission depends on the narrow authorizer and
// the permission vocabulary. It must not reach for concrete auth, handlers,
// or persistence to make an authorization decision.
func TestEnforcingServicesDoNotDependOnAuthTransportOrStores(t *testing.T) {
	forbidden := []string{"service/auth", "transport", "stores"}
	enforcing := map[string]bool{}
	sources := productionSources(t)
	for _, source := range sources {
		if source.dir == "internal/service/permission" || !strings.HasPrefix(source.dir, "internal/service/") {
			continue
		}
		if slices.Contains(source.imports, "service/permission") {
			enforcing[source.dir] = true
		}
	}
	if !enforcing["internal/service/project"] {
		t.Fatal("expected the project service to be an enforcing service; the scan is not seeing the imports")
	}
	for _, source := range sources {
		if !enforcing[source.dir] {
			continue
		}
		if imported, bad := importsAny(source.imports, forbidden...); bad {
			t.Errorf("%s imports %s; an enforcing service must depend on the narrow Authorizer instead", source.path, imported)
		}
	}
}

// Only reviewed entry points may mint a system actor or attach an actor to a
// context. Adding a caller means adding it here, which puts it in review.
func TestActorConstructorsAreCalledOnlyFromReviewedEntryPoints(t *testing.T) {
	allowed := map[string][]string{
		"ContextWithSystemActor": {
			"internal/service/agent_project_resolver.go",     // agent runs start containers from background contexts
			"internal/service/chat_notification_audience.go", // notification fan-out reads members
			"internal/service/services.go",                   // application container readiness
			"internal/service/user_removal_cleanup.go",       // removing a user revokes their access
			"internal/service/permission/assignments.go",     // RemoveUserPolicy cleanup
		},
		"SystemActor": {
			"internal/service/permission/actor.go",
		},
		"ContextWithActor": {
			"internal/transport/http/middleware/auth.go", // attaches the authenticated session's actor
			"internal/service/permission/actor.go",
		},
	}

	found := map[string][]string{}
	for _, source := range productionSources(t) {
		ast.Inspect(source.file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			case *ast.Ident:
				name = fun.Name
			}
			if _, guarded := allowed[name]; guarded {
				if !slices.Contains(found[name], source.path) {
					found[name] = append(found[name], source.path)
				}
			}
			return true
		})
	}

	for name, callers := range found {
		for _, caller := range callers {
			if !slices.Contains(allowed[name], caller) {
				t.Errorf("%s calls %s; only reviewed entry points may (see architecture_test.go)", caller, name)
			}
		}
	}
	for name, callers := range allowed {
		for _, caller := range callers {
			if !slices.Contains(found[name], caller) {
				t.Errorf("%s is allowlisted to call %s but does not; remove the stale entry", caller, name)
			}
		}
	}
}

// There is deliberately no runtime way to create a permission definition.
func TestNoRuntimeOperationCreatesPermissionDefinitions(t *testing.T) {
	for _, source := range productionSources(t) {
		if !strings.HasPrefix(source.path, "internal/service/permission/") &&
			!strings.HasPrefix(source.path, "internal/stores/filepermissions/") {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			decl, ok := node.(*ast.FuncDecl)
			if !ok {
				return true
			}
			name := strings.ToLower(decl.Name.Name)
			if (strings.Contains(name, "create") || strings.Contains(name, "register") || strings.Contains(name, "define")) &&
				strings.Contains(name, "permission") {
				t.Errorf("%s declares %s; permission definitions are created only in code", source.path, decl.Name.Name)
			}
			return true
		})
	}
}
