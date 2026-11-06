package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/containerio"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type call struct {
	container, command string
	request            containerio.Request
	limit              int64
}

type fakeContainer struct {
	mu     sync.Mutex
	calls  []call
	output map[string]string
	err    error
}

func (f *fakeContainer) run(_ context.Context, container, command string, request containerio.Request, limit int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{container, command, request, limit})
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.output[command]), nil
}

func newTestAPI(t *testing.T, container *fakeContainer) *API {
	t.Helper()
	b := New(container.run)
	if err := b.Init(applications.Instance{Scope: "project", ContainerName: "project-demo"}); err != nil {
		t.Fatal(err)
	}
	return b
}

func get(t *testing.T, b *API, path string, query map[string][]string) applications.Response {
	t.Helper()
	response, err := b.Handle(applications.Request{Method: "GET", Path: path, Query: query})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestRoutesRunTheContainerProgramWithStdinRequests(t *testing.T) {
	container := &fakeContainer{output: map[string]string{
		"list":    `{"path":"src","entries":[{"name":"app.go","path":"src/app.go","isDir":false,"size":12,"extra":"<script>"}],"truncated":true}`,
		"search":  `{"entries":null,"truncated":false}`,
		"archive": "PK zip bytes",
	}}
	b := newTestAPI(t, container)

	response := get(t, b, "list", map[string][]string{"path": {"src"}})
	if response.Status != 200 || strings.Contains(string(response.Body), "extra") {
		t.Fatalf("list = %d %s", response.Status, response.Body)
	}
	var listed listing
	if err := json.Unmarshal(response.Body, &listed); err != nil || listed.Path != "src" || !listed.Truncated ||
		len(listed.Entries) != 1 || listed.Entries[0].Size != 12 {
		t.Fatalf("listing = %+v, %v", listed, err)
	}

	response = get(t, b, "search", map[string][]string{"q": {"app"}})
	if response.Status != 200 || string(response.Body) != `{"entries":[],"truncated":false}` {
		t.Fatalf("search = %d %s", response.Status, response.Body)
	}

	response = get(t, b, "archive", map[string][]string{"path": {"/src/../src/"}})
	if response.Status != 200 || string(response.Body) != "PK zip bytes" ||
		response.Headers["Content-Type"][0] != "application/zip" ||
		response.Headers["Content-Disposition"][0] != `attachment; filename=src.zip` {
		t.Fatalf("archive = %d %v %s", response.Status, response.Headers, response.Body)
	}
	if response := get(t, b, "archive", nil); response.Headers["Content-Disposition"][0] != `attachment; filename=workspace.zip` {
		t.Fatalf("workspace archive = %v", response.Headers)
	}

	want := []call{
		{"project-demo", "list", containerio.Request{Path: "src"}, 32 << 20},
		{"project-demo", "search", containerio.Request{Query: "app"}, 32 << 20},
		{"project-demo", "archive", containerio.Request{Path: "src"}, 256 << 20},
		{"project-demo", "archive", containerio.Request{}, 256 << 20},
	}
	if len(container.calls) != len(want) {
		t.Fatalf("calls = %+v", container.calls)
	}
	for i := range want {
		if container.calls[i] != want[i] {
			t.Fatalf("call %d = %+v, want %+v", i, container.calls[i], want[i])
		}
	}
}

func TestContainerFailuresMapToHTTPStatuses(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{containerio.ErrInvalidPath, 400},
		{containerio.ErrNotFound, 404},
		{containerio.ErrTooLarge, 413},
		{context.DeadlineExceeded, 504},
		{context.Canceled, 502},
	} {
		b := newTestAPI(t, &fakeContainer{err: test.err})
		if response := get(t, b, "archive", map[string][]string{"path": {"src"}}); response.Status != test.status {
			t.Errorf("%v: status %d, want %d", test.err, response.Status, test.status)
		}
	}
	b := newTestAPI(t, &fakeContainer{output: map[string]string{"list": "not json"}})
	if response := get(t, b, "list", nil); response.Status != 502 {
		t.Fatalf("unreadable listing = %d", response.Status)
	}
}

func TestRequestsNeedAContainerAndBoundedInput(t *testing.T) {
	container := &fakeContainer{}
	b := New(container.run)
	if response := get(t, b, "list", nil); response.Status != 409 {
		t.Fatalf("without a container = %d", response.Status)
	}
	b = newTestAPI(t, container)
	if response := get(t, b, "search", map[string][]string{"q": {strings.Repeat("a", 4097)}}); response.Status != 400 {
		t.Fatalf("oversized query = %d", response.Status)
	}
	if len(container.calls) != 0 {
		t.Fatalf("container was called: %+v", container.calls)
	}
}

func TestArchivesAreBoundedPerInstallation(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 3)
	b := New(func(context.Context, string, string, containerio.Request, int64) ([]byte, error) {
		started <- struct{}{}
		<-release
		return []byte("zip"), nil
	})
	_ = b.Init(applications.Instance{ContainerName: "project-demo"})
	var done sync.WaitGroup
	for range 3 {
		done.Add(1)
		go func() {
			defer done.Done()
			get(t, b, "archive", nil)
		}()
	}
	for range 2 {
		<-started
	}
	select {
	case <-started:
		t.Fatal("a third archive started while two were running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-started
	done.Wait()
}
