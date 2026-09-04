package selfupdate

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const (
	stateDirName = "self-update"
	logTailBytes = 16 * 1024
	runFileMode  = 0o600
)

// runState owns the durable files that survive an updater-triggered backend
// restart. Update selection and launch policy remain in Service.
type runState struct {
	dir string
}

func newRunState(dataDir string) runState {
	return runState{dir: filepath.Join(dataDir, stateDirName)}
}

// reset removes the previous result and prepares an empty log for a fresh run.
func (r runState) reset() error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	if err := removeIfPresent(r.donePath()); err != nil {
		return err
	}
	if err := removeIfPresent(r.progressPath()); err != nil {
		return err
	}
	return os.WriteFile(r.logPath(), nil, runFileMode)
}

func (r runState) launch(installDir, target string, kind UpdateKind) UpdaterLaunch {
	return UpdaterLaunch{
		InstallDir:   installDir,
		Target:       target,
		Kind:         kind,
		LogPath:      r.logPath(),
		DonePath:     r.donePath(),
		ProgressPath: r.progressPath(),
	}
}

func (r runState) writeProgress(progress Progress) error {
	return writeJSONFile(r.progressPath(), progress)
}

func (r runState) removeProgress() {
	_ = os.Remove(r.progressPath())
}

func (r runState) writeRecord(record runRecord) error {
	return writeJSONFile(r.runPath(), record)
}

// status reconstructs the last run from disk: the done marker wins, a live
// PID means running, and a dead PID without a marker means the run crashed
// before it could report.
func (r runState) status(processAlive func(int) bool) *RunStatus {
	var record runRecord
	if err := readJSONFile(r.runPath(), &record); err != nil {
		return nil
	}
	logText, logUpdatedAt := readLog(r.logPath(), logTailBytes)
	status := &RunStatus{
		State:        "running",
		Target:       record.Target,
		UpdateKind:   record.UpdateKind,
		StartedAt:    record.StartedAt,
		StartedBy:    record.StartedBy,
		Log:          stripANSI(logText),
		LogUpdatedAt: logUpdatedAt,
	}
	var progress Progress
	if err := readJSONFile(r.progressPath(), &progress); err == nil && progress.Phase != "" {
		status.Progress = &progress
	}
	var done doneRecord
	switch err := readJSONFile(r.donePath(), &done); {
	case err == nil:
		status.FinishedAt = done.FinishedAt
		status.ExitCode = &done.ExitCode
		if done.ExitCode == 0 {
			status.State = "succeeded"
		} else {
			status.State = "failed"
		}
	case !processAlive(record.PID):
		status.State = "failed"
		status.Log += "\n(updater process exited without reporting a result)"
	}
	return status
}

func (r runState) runPath() string      { return filepath.Join(r.dir, "run.json") }
func (r runState) donePath() string     { return filepath.Join(r.dir, "done.json") }
func (r runState) logPath() string      { return filepath.Join(r.dir, "run.log") }
func (r runState) progressPath() string { return filepath.Join(r.dir, "progress.json") }

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func writeJSONFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, runFileMode)
}

// readLog returns up to the last max bytes and the file's modification time.
func readLog(path string, max int64) (string, int64) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0
	}
	if size := info.Size(); size > max {
		if _, err := file.Seek(size-max, io.SeekStart); err != nil {
			return "", 0
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", 0
	}
	return string(data), info.ModTime().Unix()
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(value string) string {
	return ansiEscape.ReplaceAllString(value, "")
}
