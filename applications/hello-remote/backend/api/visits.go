package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// visitsFile is where the greeting counter lives inside the instance's
// DataDir. That directory is the only storage a backend can rely on: the
// process is killed on stop, uninstall, and server restart, and restarted
// lazily by the next call, so anything kept in memory is gone by then. The
// counter surviving a restart is the whole point of the example.
const visitsFile = "visits.json"

func (b *api) readVisits(applications.Request) applications.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	return applications.JSON(http.StatusOK, map[string]int{"visits": b.visits})
}

func (b *api) countVisit(applications.Request) applications.Response {
	b.mu.Lock()
	b.visits++
	visits := b.visits
	persistErr := writeVisits(b.instance.DataDir, visits)
	b.mu.Unlock()

	warnings := make([]string, 0, 2)
	if persistErr != nil {
		// The count is still correct in memory, so the call succeeds and the
		// browser sees it; only its survival across a restart is lost. A
		// backend's errors are its own to grade — the host only forwards them.
		warnings = append(warnings, fmt.Sprintf("not persisted: %v", persistErr))
	}

	// Never hold application state locks while emitting an event into core. The
	// runtime boundary does not belong inside the counter's critical section.
	publicationErr := b.greetings.Greeted(visits)
	if publicationErr != nil {
		// Publishing is an observable side effect, not the greeting operation's
		// transaction. The count remains successful and the warning tells the UI
		// exactly which secondary action was lost.
		warnings = append(warnings, fmt.Sprintf("event not published: %v", publicationErr))
	}

	body := map[string]any{"visits": visits}
	if len(warnings) > 0 {
		body["warning"] = strings.Join(warnings, "; ")
	}
	return applications.JSON(http.StatusOK, body)
}

// readVisits tolerates every kind of missing: no DataDir, no file, or a file
// this version cannot read. A fresh install and an unreadable one both start
// at zero rather than failing Init, which would fail the app's start.
func readVisits(dataDir string) int {
	if dataDir == "" {
		return 0
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, visitsFile))
	if err != nil {
		return 0
	}
	var state struct {
		Visits int `json:"visits"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return 0
	}
	return state.Visits
}

func writeVisits(dataDir string, visits int) error {
	if dataDir == "" {
		return fmt.Errorf("no data directory")
	}
	raw, err := json.Marshal(map[string]int{"visits": visits})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, visitsFile), raw, 0o600)
}
