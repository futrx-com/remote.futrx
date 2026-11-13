package files

import (
	"context"
	"io"
	"path"
	"path/filepath"
	"strings"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/port/files/outbound"
)

type Service struct {
	root     string
	store    outbound.Store
	archives outbound.ArchiveDownloads
}

func New(root string, store outbound.Store, archives outbound.ArchiveDownloads) *Service {
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

// PublishArchive builds the bounded archive into the workspace and returns
// where it landed. The browser downloads it from there, so the archive's bytes
// never pass through this program's output or the host backend.
func (s *Service) PublishArchive(ctx context.Context, relativePath string) (model.PublishedArchive, error) {
	archive, err := s.prepareArchive(relativePath)
	if err != nil {
		return model.PublishedArchive{}, err
	}
	published, err := s.archives.Publish(ctx, archive.Name, func(destination io.Writer) error {
		return s.store.WriteArchive(ctx, archive.Root, archive.Relative, destination)
	})
	if err != nil {
		return model.PublishedArchive{}, err
	}
	return model.PublishedArchive{Path: published}, nil
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
