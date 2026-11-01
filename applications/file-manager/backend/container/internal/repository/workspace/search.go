package workspace

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

func (s *WorkspaceFileStore) Search(root, query string, limit int) ([]*model.Node, bool, error) {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return nil, false, err
	}
	defer workspace.close()
	needle := strings.ToLower(query)

	var results []*model.Node
	truncated := false
	visits := 0
	walkErr := fs.WalkDir(workspace.root.FS(), ".", func(walkPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if walkPath == "." {
			return nil
		}
		visits++
		if visits > config.MaxSearchVisits {
			truncated = true
			return filepath.SkipAll
		}
		if !strings.Contains(strings.ToLower(entry.Name()), needle) {
			return nil
		}
		node, ok := workspace.nodeFor(path.Dir(walkPath), entry)
		if !ok {
			return nil
		}
		results = append(results, node)
		if len(results) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return nil, false, walkErr
	}
	return results, truncated, nil
}
