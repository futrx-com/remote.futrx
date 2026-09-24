package workspacepath

import "testing"

func TestResolveFile(t *testing.T) {
	workspaceRoot := "/var/lib/remote/projects/graphixy-ai/workspace"
	tests := []struct {
		name       string
		rawPath    string
		cwd        string
		wantFile   string
		wantRoot   string
		wantLine   int
		wantColumn int
	}{
		{name: "container workspace file", rawPath: "/workspace/app-graphixy-full-page-4k.png", cwd: workspaceRoot, wantFile: workspaceRoot + "/app-graphixy-full-page-4k.png", wantRoot: workspaceRoot},
		{name: "container workspace from nested cwd", rawPath: "/workspace/src/App.tsx", cwd: workspaceRoot + "/src/components", wantFile: workspaceRoot + "/src/App.tsx", wantRoot: workspaceRoot},
		{name: "container workspace file with line", rawPath: "/workspace/src/App.tsx:87", cwd: workspaceRoot, wantFile: workspaceRoot + "/src/App.tsx", wantRoot: workspaceRoot, wantLine: 87},
		{name: "container workspace file with line and column", rawPath: "/workspace/src/App.tsx:87:5", cwd: workspaceRoot, wantFile: workspaceRoot + "/src/App.tsx", wantRoot: workspaceRoot, wantLine: 87, wantColumn: 5},
		{name: "host workspace file", rawPath: workspaceRoot + "/assets/logo.png", cwd: workspaceRoot, wantFile: workspaceRoot + "/assets/logo.png", wantRoot: workspaceRoot},
		{name: "workspace root", rawPath: "/workspace", cwd: workspaceRoot, wantFile: workspaceRoot, wantRoot: workspaceRoot},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveFile(test.rawPath, test.cwd)
			if err != nil {
				t.Fatalf("ResolveFile() error = %v", err)
			}
			if got.FilePath != test.wantFile || got.WorkspaceRoot != test.wantRoot || got.Line != test.wantLine || got.Column != test.wantColumn {
				t.Fatalf("ResolveFile() = %#v", got)
			}
		})
	}
}

func TestResolveFileRejectsUnsafePaths(t *testing.T) {
	workspaceRoot := "/var/lib/remote/projects/graphixy-ai/workspace"
	for _, rawPath := range []string{"", "relative/file.txt", "/workspace/../etc/passwd", "/var/lib/remote/projects/graphixy-ai/secret.txt", "/etc/passwd"} {
		t.Run(rawPath, func(t *testing.T) {
			if got, err := ResolveFile(rawPath, workspaceRoot); err == nil {
				t.Fatalf("ResolveFile() = (%#v, nil), want error", got)
			}
		})
	}
}

func TestResolveFileAtRootPreservesExactTrustedRoot(t *testing.T) {
	t.Parallel()
	root := "/srv/workspace/remote.futrx"
	target, err := ResolveFileAtRoot("/workspace/frontend/src/main.ts", root)
	if err != nil {
		t.Fatalf("ResolveFileAtRoot: %v", err)
	}
	if target.WorkspaceRoot != root {
		t.Fatalf("workspace root = %q, want exact trusted root %q", target.WorkspaceRoot, root)
	}
	if want := root + "/frontend/src/main.ts"; target.FilePath != want {
		t.Fatalf("file path = %q, want %q", target.FilePath, want)
	}

	if _, err := ResolveFileAtRoot("/srv/workspace/other/secret", root); err == nil {
		t.Fatal("absolute path outside the exact root was accepted")
	}
}
