package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// taskStore owns durable task snapshots. Callers hold mu through each read or
// read-modify-commit operation; failed writes leave the live snapshot untouched.
type taskStore struct {
	mu       sync.Mutex
	instance applications.Instance
	tasks    map[string]Task
}

func (a *taskStore) load(instance applications.Instance) error {
	if err := os.MkdirAll(instance.DataDir, 0700); err != nil {
		return err
	}
	a.instance = instance
	data, err := os.ReadFile(a.filename())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err = json.Unmarshal(data, &a.tasks); err != nil {
			return fmt.Errorf("read scheduled tasks: %w", err)
		}
		if a.tasks == nil {
			a.tasks = map[string]Task{}
		}
	}
	return nil
}

// commit must be called while mu is held, before acknowledging or publishing a change.
func (a *taskStore) commit(tasks map[string]Task) error {
	if err := a.save(tasks); err != nil {
		return err
	}
	a.tasks = tasks
	return nil
}

func (a *taskStore) filename() string { return filepath.Join(a.instance.DataDir, "tasks.json") }
func (a *taskStore) save(tasks map[string]Task) error {
	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(a.instance.DataDir, "tasks-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), a.filename()); err != nil {
		return err
	}
	dir, err := os.Open(a.instance.DataDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
