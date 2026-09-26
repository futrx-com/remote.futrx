package workspaceide

import (
	"net/url"
	"strings"
	"testing"
)

const (
	testBaseURL      = "https://code.remote.futrx.com/"
	testProjectURL   = "https://remote.futrx.com/"
	testProjectsRoot = "/var/lib/remote/projects"
	projectWorkspace = "/var/lib/remote/projects/graphixy-ai/workspace"
)

func redirectQuery(t *testing.T, rawURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", rawURL, err)
	}
	return parsed.Query()
}

func TestOpenURLMapsProjectPathsIntoContainer(t *testing.T) {
	service := New(testBaseURL, testProjectURL, testProjectsRoot)
	got, err := service.OpenURL(projectWorkspace, "/workspace/src/App.tsx:87:5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, testProjectURL+"graphixy-ai/code/?") {
		t.Fatalf("OpenURL() = %q, want project IDE prefix %q", got, testProjectURL+"graphixy-ai/code/?")
	}

	query := redirectQuery(t, got)
	if folder := query.Get("folder"); folder != "/workspace" {
		t.Fatalf("folder = %q, want /workspace", folder)
	}
	if file := query.Get("file"); file != "" {
		t.Fatalf("legacy file parameter should be gone, got %q", file)
	}
	// The workbench payload is what actually opens the file: the line:column
	// suffix plus gotoLineMode place the cursor (verified on code-server 4.121.0).
	payload := query.Get("payload")
	for _, want := range []string{
		`["openFile","vscode-remote://remote.futrx.com/workspace/src/App.tsx:87:5"]`,
		`["gotoLineMode","true"]`,
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("payload %q missing %q", payload, want)
		}
	}
}

func TestOpenURLLineWithoutColumn(t *testing.T) {
	service := New(testBaseURL, testProjectURL, testProjectsRoot)
	got, err := service.OpenURL(projectWorkspace, "/workspace/docs/flow.md:92")
	if err != nil {
		t.Fatal(err)
	}
	payload := redirectQuery(t, got).Get("payload")
	if !strings.Contains(payload, "/workspace/docs/flow.md:92\"") {
		t.Fatalf("payload %q missing bare line suffix", payload)
	}
}

func TestOpenURLFileWithoutLineOmitsGotoLineMode(t *testing.T) {
	service := New(testBaseURL, testProjectURL, testProjectsRoot)
	got, err := service.OpenURL(projectWorkspace, "/workspace/README.md")
	if err != nil {
		t.Fatal(err)
	}
	payload := redirectQuery(t, got).Get("payload")
	if !strings.Contains(payload, `["openFile","vscode-remote://remote.futrx.com/workspace/README.md"]`) {
		t.Fatalf("payload %q missing plain openFile entry", payload)
	}
	// Without gotoLineMode the workbench keeps colon-bearing paths intact, so
	// only line-suffixed opens may enable it.
	if strings.Contains(payload, "gotoLineMode") {
		t.Fatalf("payload %q must not enable gotoLineMode without a line", payload)
	}
}

func TestOpenURLWorkspaceRootHasNoPayload(t *testing.T) {
	service := New(testBaseURL, testProjectURL, testProjectsRoot)
	got, err := service.OpenURL(projectWorkspace, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	query := redirectQuery(t, got)
	if payload := query.Get("payload"); payload != "" {
		t.Fatalf("opening the workspace root should not carry a payload: %q", got)
	}
	if folder := query.Get("folder"); folder != "/workspace" {
		t.Fatalf("folder = %q, want /workspace", folder)
	}
}

func TestOpenFilePayloadEscapesPaths(t *testing.T) {
	payload := openFilePayload("code.remote.futrx.com", "/workspace/a b/file.md", 12, 0)
	for _, want := range []string{
		"vscode-remote://code.remote.futrx.com/workspace/a%20b/file.md:12",
		`["gotoLineMode","true"]`,
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("payload %q missing %q", payload, want)
		}
	}
}
