package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// notes is the same demonstration one level down: DataDir is the plugin's own
// directory on the host, so what it writes there survives a restart and is
// deleted only when the app is uninstalled.
func (p *playground) notesPath() (string, error) {
	p.mu.Lock()
	dataDir := p.instance.DataDir
	p.mu.Unlock()
	if dataDir == "" {
		return "", fmt.Errorf("no data directory was assigned")
	}
	return filepath.Join(dataDir, "note.json"), nil
}

func (p *playground) readNotes(appplugin.Request) appplugin.Response {
	path, err := p.notesPath()
	if err != nil {
		return appplugin.Errorf(http.StatusInternalServerError, "%v", err)
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return appplugin.JSON(http.StatusOK, map[string]any{"note": "", "path": path, "saved": false})
	}
	if err != nil {
		return appplugin.Errorf(http.StatusInternalServerError, "read note: %v", err)
	}
	var stored struct {
		Note      string `json:"note"`
		WrittenAt string `json:"writtenAt"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return appplugin.Errorf(http.StatusInternalServerError, "stored note is unreadable: %v", err)
	}
	return appplugin.JSON(http.StatusOK, map[string]any{
		"note":      stored.Note,
		"writtenAt": stored.WrittenAt,
		"path":      path,
		"saved":     true,
	})
}

func (p *playground) writeNotes(request appplugin.Request) appplugin.Response {
	path, err := p.notesPath()
	if err != nil {
		return appplugin.Errorf(http.StatusInternalServerError, "%v", err)
	}
	var body struct {
		Note string `json:"note"`
	}
	if err := request.DecodeJSON(&body); err != nil {
		return appplugin.Errorf(http.StatusBadRequest, "invalid json: %v", err)
	}
	raw, err := json.Marshal(map[string]string{
		"note":      body.Note,
		"writtenAt": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return appplugin.Errorf(http.StatusInternalServerError, "encode note: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return appplugin.Errorf(http.StatusInternalServerError, "write note: %v", err)
	}
	return appplugin.JSON(http.StatusOK, map[string]any{"note": body.Note, "path": path, "saved": true})
}
