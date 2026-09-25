// Package lifecycle owns S3Disk's per-instance mount operation lifecycle.
// Remote owns the installed service lifecycle declared in application.json.
package lifecycle

import "sync"

// Result is the visible outcome of a completed sync or restart.
type Result struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Operation is the latest operation reported to the S3Disk UI.
type Operation struct {
	Action  string `json:"action"`
	Running bool   `json:"running"`
	Result  Result `json:"result"`
}

// MountOperations is the API-facing contract for per-instance operations.
type MountOperations interface {
	Snapshot() Operation
	Start(action string, run func() Result) (Operation, bool)
}

// Operations keeps at most one sync or restart running per installed instance.
type Operations struct {
	mu      sync.Mutex
	current Operation
}

var _ MountOperations = (*Operations)(nil)

func NewOperations() *Operations { return &Operations{} }

func (s *Operations) Snapshot() Operation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Start records the running operation before launching its work. A concurrent
// call observes that state and is rejected until the work finishes.
func (s *Operations) Start(action string, run func() Result) (Operation, bool) {
	s.mu.Lock()
	if s.current.Running {
		s.mu.Unlock()
		return Operation{}, false
	}
	s.current = Operation{Action: action, Running: true}
	started := s.current
	s.mu.Unlock()

	go func() {
		result := run()
		s.mu.Lock()
		s.current = Operation{Action: action, Result: result}
		s.mu.Unlock()
	}()
	return started, true
}
