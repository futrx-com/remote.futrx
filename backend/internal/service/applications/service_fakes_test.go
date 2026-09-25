package applications

import (
	"context"
	"sync"
)

// singleApplicationRegistry answers with one application, so a test can shape
// exactly the application under test rather than the whole catalog.
type singleApplicationRegistry struct{ application Application }

func (r *singleApplicationRegistry) List() []Application { return []Application{r.application} }

func (r *singleApplicationRegistry) Get(id string) (Application, bool) {
	if id != r.application.ID {
		return Application{}, false
	}
	return r.application, true
}

func (r *singleApplicationRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

// fakeStore is the shared in-memory store for application service tests. It
// records writes and deletions while keeping reads consistent with them.
type fakeStore struct {
	mu         sync.RWMutex
	global     []Instance
	byProject  map[string][]Instance
	listAllErr error
	deleteErr  error

	puts    []Instance
	deleted []string
}

func (f *fakeStore) ListGlobal(context.Context) ([]Instance, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]Instance(nil), f.global...), nil
}

func (f *fakeStore) ListProject(_ context.Context, projectID string) ([]Instance, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]Instance(nil), f.byProject[projectID]...), nil
}

// ListAll spans both scopes, as the real store does. A caller asking "is this
// application installed anywhere" gets the wrong answer from a fake that only knows
// about global instances.
func (f *fakeStore) ListAll(context.Context) ([]Instance, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.listAllErr != nil {
		return nil, f.listAllErr
	}
	all := append([]Instance(nil), f.global...)
	for _, group := range projectInstanceGroups(f.byProject) {
		all = append(all, group...)
	}
	return all, nil
}

// Get scans the lists the fixture was built from, so a test can hand the
// service an instance without a second source of truth for it.
func (f *fakeStore) Get(_ context.Context, id string) (Instance, bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, group := range append([][]Instance{f.global}, projectInstanceGroups(f.byProject)...) {
		for _, candidate := range group {
			if candidate.ID == id {
				return candidate, true, nil
			}
		}
	}
	return Instance{}, false, nil
}

// Put upserts, so a service that installs and then acts on what it installed
// sees one instance rather than two.
func (f *fakeStore) Put(_ context.Context, instance Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts = append(f.puts, instance)
	group := &f.global
	if instance.Scope == ScopeProject {
		if f.byProject == nil {
			f.byProject = map[string][]Instance{}
		}
		existing := f.byProject[instance.ProjectID]
		group = &existing
		defer func() { f.byProject[instance.ProjectID] = *group }()
	}
	for i, candidate := range *group {
		if candidate.ID == instance.ID {
			(*group)[i] = instance
			return nil
		}
	}
	*group = append(*group, instance)
	return nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	kept := f.global[:0]
	for _, candidate := range f.global {
		if candidate.ID != id {
			kept = append(kept, candidate)
		}
	}
	f.global = kept
	return nil
}

func projectInstanceGroups(byProject map[string][]Instance) [][]Instance {
	groups := make([][]Instance, 0, len(byProject))
	for _, group := range byProject {
		groups = append(groups, group)
	}
	return groups
}
