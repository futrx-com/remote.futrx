package templates

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadShippedCatalog(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	want := []string{"blank", "wordpress", "laravel", "node", "python"}
	got := catalog.Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
	if catalog.DefaultName() != DefaultName {
		t.Fatalf("DefaultName() = %q, want %q", catalog.DefaultName(), DefaultName)
	}
}

func TestShippedTemplateProperties(t *testing.T) {
	catalog := MustLoad()

	tests := []struct {
		name           string
		wantProvisions bool
		wantImage      string
		wantPorts      []int
	}{
		{name: "blank", wantProvisions: false, wantImage: ""},
		{name: "wordpress", wantProvisions: true, wantImage: "futrx-remote-wordpress-base", wantPorts: []int{8080}},
		{name: "laravel", wantProvisions: true, wantImage: "futrx-remote-laravel-base", wantPorts: []int{8000, 5173}},
		{name: "node", wantProvisions: true, wantImage: "", wantPorts: []int{3000, 5173}},
		{name: "python", wantProvisions: true, wantImage: "", wantPorts: []int{8000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template, ok := catalog.Get(tt.name)
			if !ok {
				t.Fatalf("Get(%q) missing", tt.name)
			}
			if template.Provisions() != tt.wantProvisions {
				t.Fatalf("Provisions() = %t, want %t", template.Provisions(), tt.wantProvisions)
			}
			if template.ImageAlias() != tt.wantImage {
				t.Fatalf("ImageAlias() = %q, want %q", template.ImageAlias(), tt.wantImage)
			}
			if len(template.DefaultPorts) != len(tt.wantPorts) {
				t.Fatalf("DefaultPorts = %v, want %v", template.DefaultPorts, tt.wantPorts)
			}
			for i := range tt.wantPorts {
				if template.DefaultPorts[i] != tt.wantPorts[i] {
					t.Fatalf("DefaultPorts = %v, want %v", template.DefaultPorts, tt.wantPorts)
				}
			}
			if tt.wantProvisions && len(template.Instructions) == 0 {
				t.Fatal("a provisioning template must ship agent instructions")
			}
		})
	}
}

func TestGetIsCaseInsensitiveAndResolveFallsBackToDefault(t *testing.T) {
	catalog := MustLoad()

	if _, ok := catalog.Get("  WordPress "); !ok {
		t.Fatal("Get should normalize the requested name")
	}
	if !catalog.Has("PYTHON") {
		t.Fatal("Has should normalize the requested name")
	}
	if catalog.Has("does-not-exist") {
		t.Fatal("Has should reject an unknown template")
	}
	// A project created from a template that a later release dropped must keep
	// starting; it degrades to the template that installs nothing.
	if got := catalog.Resolve("retired-stack").Name; got != DefaultName {
		t.Fatalf("Resolve(unknown) = %q, want %q", got, DefaultName)
	}
	if got := catalog.Resolve("").Name; got != DefaultName {
		t.Fatalf("Resolve(\"\") = %q, want %q", got, DefaultName)
	}
}

func TestLoadFSRejectsMalformedTemplates(t *testing.T) {
	valid := `{"name":"blank","title":"Blank","description":"d","icon":"blank"}`

	tests := []struct {
		name    string
		files   fstest.MapFS
		wantErr string
	}{
		{
			name: "missing default template",
			files: fstest.MapFS{
				"node/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"node","title":"Node","description":"d","icon":"node"}`),
				},
			},
			wantErr: "default template",
		},
		{
			name: "name does not match directory",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{Data: []byte(valid)},
				"other/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"node","title":"Node","description":"d","icon":"node"}`),
				},
			},
			wantErr: "does not match its directory",
		},
		{
			name: "invalid name",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{Data: []byte(valid)},
				"Bad_Name/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"Bad_Name","title":"x","description":"d","icon":"i"}`),
				},
			},
			wantErr: "must match",
		},
		{
			name: "missing title",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":" ","description":"d","icon":"i"}`),
				},
			},
			wantErr: "title is required",
		},
		{
			name: "unknown field",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i","typo":1}`),
				},
			},
			wantErr: "unknown field",
		},
		{
			name: "port out of range",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i","defaultPorts":[70000]}`),
				},
			},
			wantErr: "out of range",
		},
		{
			name: "seed escapes the workspace",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i",` +
						`"seedFiles":[{"source":"seed.txt","target":"/etc/passwd"}]}`),
				},
				"blank/seed.txt": &fstest.MapFile{Data: []byte("x")},
			},
			wantErr: "under /workspace",
		},
		{
			name: "seed traverses upwards",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i",` +
						`"seedFiles":[{"source":"seed.txt","target":"/workspace/../etc/x"}]}`),
				},
				"blank/seed.txt": &fstest.MapFile{Data: []byte("x")},
			},
			wantErr: "clean absolute path",
		},
		{
			name: "duplicate seed target",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i",` +
						`"seedFiles":[{"source":"a.txt","target":"/workspace/x"},` +
						`{"source":"b.txt","target":"/workspace/x"}]}`),
				},
				"blank/a.txt": &fstest.MapFile{Data: []byte("a")},
				"blank/b.txt": &fstest.MapFile{Data: []byte("b")},
			},
			wantErr: "declared twice",
		},
		{
			name: "missing provision script",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i","provisionScript":"gone.sh"}`),
				},
			},
			wantErr: "provision script",
		},
		{
			name: "script escapes the template directory",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i","provisionScript":"../x.sh"}`),
				},
			},
			wantErr: "relative file name",
		},
		{
			name: "empty seed file",
			files: fstest.MapFS{
				"blank/template.json": &fstest.MapFile{
					Data: []byte(`{"name":"blank","title":"t","description":"d","icon":"i",` +
						`"seedFiles":[{"source":"a.txt","target":"/workspace/x"}]}`),
				},
				"blank/a.txt": &fstest.MapFile{Data: []byte("")},
			},
			wantErr: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFS(tt.files)
			if err == nil {
				t.Fatal("LoadFS() = nil error, want failure")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadFS() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFSResolvesReferencedFiles(t *testing.T) {
	files := fstest.MapFS{
		"blank/template.json": &fstest.MapFile{
			Data: []byte(`{"name":"blank","title":"Blank","description":"d","icon":"i"}`),
		},
		"stack/template.json": &fstest.MapFile{
			Data: []byte(`{"name":"stack","title":"Stack","description":"d","icon":"i",` +
				`"provisionScript":"provision.sh","agentInstructions":"AGENTS.md",` +
				`"seedFiles":[{"source":"README.md","target":"/workspace/README.md","mode":"600"}]}`),
		},
		"stack/provision.sh": &fstest.MapFile{Data: []byte("echo hi\n")},
		"stack/AGENTS.md":    &fstest.MapFile{Data: []byte("# stack\n")},
		"stack/README.md":    &fstest.MapFile{Data: []byte("readme\n")},
	}

	catalog, err := LoadFS(files)
	if err != nil {
		t.Fatalf("LoadFS() = %v", err)
	}
	template, ok := catalog.Get("stack")
	if !ok {
		t.Fatal("Get(stack) missing")
	}
	if string(template.Script) != "echo hi\n" {
		t.Fatalf("Script = %q", template.Script)
	}
	if string(template.Instructions) != "# stack\n" {
		t.Fatalf("Instructions = %q", template.Instructions)
	}
	if len(template.Seeds) != 1 ||
		template.Seeds[0].Target != "/workspace/README.md" ||
		template.Seeds[0].Mode != "600" ||
		string(template.Seeds[0].Content) != "readme\n" {
		t.Fatalf("Seeds = %+v", template.Seeds)
	}
	// The agent-instructions snippet is written as an implicit seed, after
	// the declared ones.
	seeds := template.allSeeds()
	if len(seeds) != 2 || seeds[1].Target != InstructionsPath || seeds[1].Mode != "644" {
		t.Fatalf("allSeeds() = %+v", seeds)
	}
}

func TestSeedModeDefaultsTo644(t *testing.T) {
	files := fstest.MapFS{
		"blank/template.json": &fstest.MapFile{
			Data: []byte(`{"name":"blank","title":"Blank","description":"d","icon":"i",` +
				`"seedFiles":[{"source":"a.txt","target":"/workspace/a.txt"}]}`),
		},
		"blank/a.txt": &fstest.MapFile{Data: []byte("a")},
	}
	catalog, err := LoadFS(files)
	if err != nil {
		t.Fatalf("LoadFS() = %v", err)
	}
	template, _ := catalog.Get("blank")
	if template.Seeds[0].Mode != "644" {
		t.Fatalf("Mode = %q, want 644", template.Seeds[0].Mode)
	}
}
