package s3fs

import (
	"strings"
	"sync"
)

// pendingSet holds the names that exist only in the local cache, because the
// object behind them has not been uploaded yet. A listing read from S3 cannot
// contain them, so readdir merges them in, an emptiness check counts them, and
// "no such file" is not an answer that may be given about one.
//
// A name is added when the file is created and removed when its object lands,
// is deleted, or moves.
type pendingSet struct {
	mu sync.Mutex
	m  map[string]map[string]bool // parent directory path -> names
}

func newPendingSet() *pendingSet {
	return &pendingSet{m: make(map[string]map[string]bool)}
}

func (p *pendingSet) add(dir, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	names, ok := p.m[dir]
	if !ok {
		names = make(map[string]bool)
		p.m[dir] = names
	}
	names[name] = true
}

func (p *pendingSet) remove(dir, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if names, ok := p.m[dir]; ok {
		delete(names, name)
		if len(names) == 0 {
			delete(p.m, dir)
		}
	}
}

// names lists what is still local-only in one directory.
func (p *pendingSet) names(dir string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	names := p.m[dir]
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	return out
}

// removePrefix forgets a whole subtree, for a recursive rename or delete.
func (p *pendingSet) removePrefix(prefix string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for dir := range p.m {
		if dir == prefix || strings.HasPrefix(dir, prefix+"/") {
			delete(p.m, dir)
		}
	}
}
