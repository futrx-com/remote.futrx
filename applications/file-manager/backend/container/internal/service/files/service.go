package files

import (
	"context"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/port/files/outbound"
)

type Service struct {
	root     string
	store    outbound.Store
	archives outbound.ArchiveSpool
}

func New(root string, store outbound.Store, archives outbound.ArchiveSpool) *Service {
	return &Service{root: root, store: store, archives: archives}
}

// List returns the entries directly under relativePath within the workspace.
// An empty relativePath lists the workspace root.
func (s *Service) List(relativePath string) (model.Listing, error) {
	root := s.root
	if root == "" {
		return model.Listing{}, model.ErrInvalidPath
	}
	rel := cleanRelative(relativePath)
	entries, truncated, err := s.store.ListDir(root, rel, config.MaxDirectoryEntries)
	if err != nil {
		return model.Listing{}, model.ErrFolderNotFound
	}
	if entries == nil {
		entries = []*model.Node{}
	}
	return model.Listing{Path: rel, Entries: entries, Truncated: truncated}, nil
}

func (s *Service) OpenFile(relativePath string) (*model.File, error) {
	root := s.root
	if root == "" {
		return nil, model.ErrInvalidPath
	}
	rel := cleanRelative(relativePath)
	if rel == "" {
		return nil, model.ErrInvalidPath
	}
	content, _, modTime, err := s.store.OpenFile(root, rel)
	if err != nil {
		return nil, model.ErrFileNotFound
	}
	return model.NewFile(path.Base(rel), modTime, content), nil
}

func (s *Service) prepareArchive(relativePath string) (model.Archive, error) {
	root := s.root
	if root == "" {
		return model.Archive{}, model.ErrInvalidPath
	}
	rel := cleanRelative(relativePath)
	if !s.store.DirectoryExists(root, rel) {
		return model.Archive{}, model.ErrFolderNotFound
	}
	name := path.Base(rel)
	if rel == "" || name == "." || name == "/" {
		name = "workspace"
	}
	return model.Archive{Name: name + ".zip", Root: root, Relative: rel}, nil
}

// DownloadArchive completes the bounded archive before presentation writes
// download headers, so build failures never look like successful ZIP responses.
func (s *Service) DownloadArchive(ctx context.Context, relativePath string) (*model.File, error) {
	archive, err := s.prepareArchive(relativePath)
	if err != nil {
		return nil, err
	}
	content, err := s.archives.Prepare(ctx, func(destination io.Writer) error {
		return s.store.WriteArchive(ctx, archive.Root, archive.Relative, destination)
	})
	if err != nil {
		return nil, err
	}
	return model.NewFile(archive.Name, time.Time{}, content), nil
}

func (s *Service) Search(query string) (model.SearchResult, error) {
	root := s.root
	if root == "" {
		return model.SearchResult{}, model.ErrInvalidPath
	}
	trimmed := strings.TrimSpace(query)
	if len([]rune(trimmed)) < config.MinSearchQuery {
		return model.SearchResult{Entries: []*model.Node{}}, nil
	}
	entries, truncated, err := s.store.Search(root, trimmed, config.MaxSearchResults)
	if err != nil {
		return model.SearchResult{}, model.ErrFolderNotFound
	}
	if entries == nil {
		entries = []*model.Node{}
	}
	return model.SearchResult{Entries: entries, Truncated: truncated}, nil
}

// cleanRelative normalises a caller-supplied path into a forward-slash path
// relative to the workspace root. Traversal ("..") is collapsed here; the store
// enforces the real containment guarantee (including symlinks).
func cleanRelative(relativePath string) string {
	relativePath = strings.TrimSpace(filepath.ToSlash(relativePath))
	if relativePath == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(relativePath, "/"))
	return strings.TrimPrefix(cleaned, "/")
}
