package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type workspaceProgress struct {
	Phase       string `json:"phase"`
	Message     string `json:"message"`
	Completed   int    `json:"completed,omitempty"`
	Total       int    `json:"total,omitempty"`
	CurrentItem string `json:"currentItem,omitempty"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type progressWriter struct {
	path string
	now  func() time.Time
}

func newProgressWriter(path string) progressWriter {
	return progressWriter{path: path, now: time.Now}
}

func (w progressWriter) write(completed, total int, current, message string) error {
	if w.path == "" {
		return nil
	}
	state := workspaceProgress{
		Phase:       "workspace-migration",
		Message:     message,
		Completed:   completed,
		Total:       total,
		CurrentItem: current,
		UpdatedAt:   w.now().Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(w.path), ".progress-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, w.path)
}
