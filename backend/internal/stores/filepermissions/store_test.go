package filepermissions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/permission"
)

func assignment(id, email string) permission.Assignment {
	return permission.Assignment{
		ID:         id,
		UserEmail:  email,
		Permission: "projects.lifecycle.manage",
		Effect:     permission.Deny,
		Scope:      permission.ProjectScope("abcd"),
		CreatedBy:  "admin@example.com",
		CreatedAt:  1,
	}
}

func addAssignment(a permission.Assignment) func(permission.State) (permission.State, []permission.AuditEvent, error) {
	return func(state permission.State) (permission.State, []permission.AuditEvent, error) {
		state.Assignments = append(state.Assignments, a)
		return state, []permission.AuditEvent{{
			At: 1, Actor: a.CreatedBy, Operation: permission.AuditAssignmentSet,
			TargetUser: a.UserEmail, Permission: a.Permission, Scope: a.Scope, NewEffect: a.Effect,
		}}, nil
	}
}

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return store, dir
}

func readAuditLines(t *testing.T, dir string) []auditRecord {
	t.Helper()
	file, err := os.Open(filepath.Join(dir, auditFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var records []auditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record auditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("audit line %q: %v", scanner.Text(), err)
		}
		records = append(records, record)
	}
	return records
}

func TestAbsentFileIsAnEmptyPolicy(t *testing.T) {
	store, dir := newStore(t)

	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Assignments)+len(state.Roles)+len(state.Bindings) != 0 {
		t.Fatalf("state = %#v, want empty", state)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("New() created %s without a mutation", stateFileName)
	}
}

func TestMutationPersistsAcrossRestart(t *testing.T) {
	store, dir := newStore(t)
	role := permission.Role{
		ID: "role1", Name: "Operators", Description: "Runs projects",
		Rules:     []permission.RoleRule{{Permission: "projects.lifecycle.manage", Effect: permission.Allow}},
		CreatedBy: "admin@example.com", CreatedAt: 1, UpdatedAt: 2,
	}
	binding := permission.RoleBinding{
		ID: "bind1", RoleID: "role1", UserEmail: "member@example.com",
		Scope: permission.ProjectScope("abcd"), CreatedBy: "admin@example.com", CreatedAt: 3,
	}
	err := store.Mutate(context.Background(), func(state permission.State) (permission.State, []permission.AuditEvent, error) {
		state.Assignments = append(state.Assignments, assignment("a1", "member@example.com"))
		state.Roles = append(state.Roles, role)
		state.Bindings = append(state.Bindings, binding)
		return state, []permission.AuditEvent{{At: 1, Actor: "admin@example.com", Operation: permission.AuditRoleCreated}}, nil
	})
	if err != nil {
		t.Fatalf("Mutate() error = %v", err)
	}

	reopened, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, _ := reopened.Load(context.Background())
	want, _ := store.Load(context.Background())
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("reloaded state differs:\n got %s\nwant %s", gotJSON, wantJSON)
	}
	if len(got.Assignments) != 1 || len(got.Roles) != 1 || len(got.Bindings) != 1 {
		t.Fatalf("reloaded state = %#v", got)
	}
}

func TestFilesAreOwnerOnly(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mutate(context.Background(), addAssignment(assignment("a1", "member@example.com"))); err != nil {
		t.Fatal(err)
	}

	for name, wantMode := range map[string]os.FileMode{
		"":            0o700,
		stateFileName: 0o600,
		auditFileName: 0o600,
	} {
		info, err := os.Stat(filepath.Join(dataDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != wantMode {
			t.Fatalf("%q mode = %o, want %o", name, got, wantMode)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(dataDir, ".permissions-*.tmp"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestAuditEventIsAppendedForEveryMutation(t *testing.T) {
	store, dir := newStore(t)
	for i := range 3 {
		a := assignment(fmt.Sprintf("a%d", i), fmt.Sprintf("user%d@example.com", i))
		if err := store.Mutate(context.Background(), addAssignment(a)); err != nil {
			t.Fatal(err)
		}
	}

	records := readAuditLines(t, dir)
	if len(records) != 3 {
		t.Fatalf("audit lines = %d, want 3", len(records))
	}
	first := records[0]
	if first.Operation != string(permission.AuditAssignmentSet) || first.Actor != "admin@example.com" ||
		first.TargetUser != "user0@example.com" || first.Permission != "projects.lifecycle.manage" ||
		first.Scope.Kind != "project" || first.Scope.ID != "abcd" || first.NewEffect != "deny" {
		t.Fatalf("first audit record = %#v", first)
	}
}

func TestFailedAuditPreventsPolicyMutation(t *testing.T) {
	store, dir := newStore(t)
	if err := store.Mutate(context.Background(), addAssignment(assignment("a1", "keep@example.com"))); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		t.Fatal(err)
	}

	store.appendAudit = func([]byte) error { return errors.New("disk full") }
	err = store.Mutate(context.Background(), addAssignment(assignment("a2", "new@example.com")))
	if !errors.Is(err, permission.ErrAuditFailed) {
		t.Fatalf("Mutate() error = %v, want ErrAuditFailed", err)
	}

	after, _ := os.ReadFile(filepath.Join(dir, stateFileName))
	if string(after) != string(before) {
		t.Fatalf("state file changed despite failed audit:\n%s", after)
	}
	state, _ := store.Load(context.Background())
	if len(state.Assignments) != 1 {
		t.Fatalf("in-memory state has %d assignments, want 1", len(state.Assignments))
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".permissions-*.tmp"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Load(context.Background()); len(got.Assignments) != 1 {
		t.Fatalf("reloaded state has %d assignments, want 1", len(got.Assignments))
	}
}

func TestNoChangeMutationWritesNothing(t *testing.T) {
	store, dir := newStore(t)

	err := store.Mutate(context.Background(), func(state permission.State) (permission.State, []permission.AuditEvent, error) {
		state.Assignments = append(state.Assignments, assignment("ignored", "x@example.com"))
		return state, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{stateFileName, auditFileName} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s written by a no-change mutation", name)
		}
	}
	if state, _ := store.Load(context.Background()); len(state.Assignments) != 0 {
		t.Fatalf("no-change mutation altered state: %#v", state)
	}
}

func TestChangeErrorAbortsMutation(t *testing.T) {
	store, dir := newStore(t)
	sentinel := errors.New("rejected")

	err := store.Mutate(context.Background(), func(state permission.State) (permission.State, []permission.AuditEvent, error) {
		return state, nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Mutate() error = %v, want %v", err, sentinel)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("state file written after a rejected change")
	}
}

func TestLoadReturnsAPrivateCopy(t *testing.T) {
	store, _ := newStore(t)
	if err := store.Mutate(context.Background(), addAssignment(assignment("a1", "member@example.com"))); err != nil {
		t.Fatal(err)
	}

	state, _ := store.Load(context.Background())
	state.Assignments[0].Effect = permission.Allow
	state.Assignments = nil

	again, _ := store.Load(context.Background())
	if len(again.Assignments) != 1 || again.Assignments[0].Effect != permission.Deny {
		t.Fatalf("store state mutated through Load: %#v", again)
	}
}

func TestConcurrentMutationsLoseNoUpdates(t *testing.T) {
	store, dir := newStore(t)
	const writers = 24

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := assignment(fmt.Sprintf("a%02d", i), fmt.Sprintf("user%02d@example.com", i))
			errs <- store.Mutate(context.Background(), addAssignment(a))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	state, _ := store.Load(context.Background())
	if len(state.Assignments) != writers {
		t.Fatalf("assignments = %d, want %d", len(state.Assignments), writers)
	}
	if lines := readAuditLines(t, dir); len(lines) != writers {
		t.Fatalf("audit lines = %d, want %d", len(lines), writers)
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Load(context.Background()); len(got.Assignments) != writers {
		t.Fatalf("reloaded assignments = %d, want %d", len(got.Assignments), writers)
	}
}

func TestStoredEmailsAreNormalizedOnLoad(t *testing.T) {
	dir := t.TempDir()
	writeState(t, dir, `{"version":1,"assignments":[{"id":"a1","userEmail":"  Member@Example.COM ",
		"permission":"projects.lifecycle.manage","effect":"deny","scope":{"kind":"project","id":"abcd"}}],
		"roles":[],"bindings":[]}`)

	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := store.Load(context.Background())
	if got := state.Assignments[0].UserEmail; got != "member@example.com" {
		t.Fatalf("email = %q, want normalized", got)
	}
}

func writeState(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptOrInvalidFilesFailStartup(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"not json", `{`},
		{"empty file", ``},
		{"unsupported version", `{"version":2}`},
		{"missing version", `{"assignments":[]}`},
		{"duplicate ids", `{"version":1,"assignments":[
			{"id":"x","userEmail":"a@example.com","permission":"p.q.r","effect":"deny","scope":{"kind":"platform"}},
			{"id":"x","userEmail":"b@example.com","permission":"p.q.r","effect":"deny","scope":{"kind":"platform"}}]}`},
		{"id shared across kinds", `{"version":1,
			"assignments":[{"id":"x","userEmail":"a@example.com","permission":"p.q.r","effect":"deny","scope":{"kind":"platform"}}],
			"roles":[{"id":"x","name":"R","rules":[]}]}`},
		{"malformed effect", `{"version":1,"assignments":[
			{"id":"a","userEmail":"a@example.com","permission":"p.q.r","effect":"maybe","scope":{"kind":"platform"}}]}`},
		{"unknown scope kind", `{"version":1,"assignments":[
			{"id":"a","userEmail":"a@example.com","permission":"p.q.r","effect":"deny","scope":{"kind":"galaxy"}}]}`},
		{"project scope without id", `{"version":1,"assignments":[
			{"id":"a","userEmail":"a@example.com","permission":"p.q.r","effect":"deny","scope":{"kind":"project"}}]}`},
		{"binding to unknown role", `{"version":1,"bindings":[
			{"id":"b","roleId":"missing","userEmail":"a@example.com","scope":{"kind":"platform"}}]}`},
		{"duplicate assignment", `{"version":1,"assignments":[
			{"id":"a","userEmail":"a@example.com","permission":"p.q.r","effect":"deny","scope":{"kind":"platform"}},
			{"id":"b","userEmail":"A@example.com","permission":"p.q.r","effect":"allow","scope":{"kind":"platform"}}]}`},
		{"empty user", `{"version":1,"assignments":[
			{"id":"a","userEmail":" ","permission":"p.q.r","effect":"deny","scope":{"kind":"platform"}}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeState(t, dir, test.content)
			_, err := New(dir)
			if err == nil {
				t.Fatal("New() error = nil, want a startup error")
			}
			if !strings.Contains(err.Error(), stateFileName) && !errors.Is(err, permission.ErrInvalidState) {
				t.Fatalf("error %q does not point at the file or ErrInvalidState", err)
			}
		})
	}
}
