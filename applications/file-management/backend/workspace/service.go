package workspace

import (
	"context"
	"io"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxDirEntries    = 10000
	maxSearchResults = 300
	minSearchQuery   = 2
)

// Service applies the user-facing limits to the rooted filesystem store. The
// root is supplied by Remote's trusted Request.Context.Chat, never by the UI.
type Service struct {
	store *Store
}

func NewService(store *Store) *Service { return &Service{store: store} }

func (s *Service) List(root, relativePath string) (Listing, error) {
	if !validRoot(root) {
		return Listing{}, ErrInvalidPath
	}
	relative := cleanRelative(relativePath)
	entries, truncated, err := s.store.ListDir(root, relative, maxDirEntries)
	if err != nil {
		return Listing{}, ErrFolderNotFound
	}
	if entries == nil {
		entries = []*Node{}
	}
	return Listing{Path: relative, Entries: entries, Truncated: truncated}, nil
}

func (s *Service) Search(root, query string) (SearchResult, error) {
	if !validRoot(root) {
		return SearchResult{}, ErrInvalidPath
	}
	trimmed := strings.TrimSpace(query)
	if len([]rune(trimmed)) < minSearchQuery {
		return SearchResult{Entries: []*Node{}}, nil
	}
	entries, truncated, err := s.store.Search(root, trimmed, maxSearchResults)
	if err != nil {
		return SearchResult{}, ErrFolderNotFound
	}
	if entries == nil {
		entries = []*Node{}
	}
	return SearchResult{Entries: entries, Truncated: truncated}, nil
}

func (s *Service) OpenFile(root, relativePath string) (*File, error) {
	if !validRoot(root) {
		return nil, ErrInvalidPath
	}
	relative := cleanRelative(relativePath)
	if relative == "" {
		return nil, ErrInvalidPath
	}
	content, _, modTime, err := s.store.OpenFile(root, relative)
	if err != nil {
		return nil, ErrFileNotFound
	}
	return &File{Name: path.Base(relative), ModTime: modTime, content: content}, nil
}

func (s *Service) OpenMedia(root, relativePath string) (*File, error) {
	file, err := s.OpenFile(root, relativePath)
	if err != nil {
		return nil, err
	}
	contentType, supported := supportedMediaType(file.Name)
	if !supported {
		_ = file.Close()
		return nil, ErrUnsupportedMedia
	}
	file.ContentType = contentType
	return file, nil
}

func (s *Service) PrepareArchive(root, relativePath string) (Archive, error) {
	if !validRoot(root) {
		return Archive{}, ErrInvalidPath
	}
	relative := cleanRelative(relativePath)
	if !s.store.DirectoryExists(root, relative) {
		return Archive{}, ErrFolderNotFound
	}
	name := path.Base(relative)
	if relative == "" || name == "." || name == "/" {
		name = "workspace"
	}
	return Archive{Name: name + ".zip", root: root, relative: relative}, nil
}

func (s *Service) WriteArchive(ctx context.Context, archive Archive, destination io.Writer) error {
	return s.store.WriteArchive(ctx, archive.root, archive.relative, destination)
}

func validRoot(root string) bool { return root != "" && filepath.IsAbs(root) }

// cleanRelative collapses traversal syntactically; Store.resolve performs the
// real containment check after following symlinks.
func cleanRelative(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(value, "/"))
	return strings.TrimPrefix(cleaned, "/")
}

func supportedMediaType(name string) (string, bool) {
	contentType, ok := mediaTypes[strings.ToLower(filepath.Ext(name))]
	return contentType, ok
}

var mediaTypes = map[string]string{
	".aac": "audio/aac", ".avif": "image/avif", ".bmp": "image/bmp",
	".flac": "audio/flac", ".gif": "image/gif", ".ico": "image/x-icon",
	".jpeg": "image/jpeg", ".jpg": "image/jpeg", ".m4a": "audio/mp4",
	".m4v": "video/mp4", ".mov": "video/quicktime", ".mp3": "audio/mpeg",
	".mp4": "video/mp4", ".oga": "audio/ogg", ".ogg": "audio/ogg",
	".ogv": "video/ogg", ".opus": "audio/opus", ".pdf": "application/pdf",
	".png": "image/png", ".svg": "image/svg+xml", ".tif": "image/tiff",
	".tiff": "image/tiff", ".wav": "audio/wav", ".webm": "video/webm",
	".webp": "image/webp",
}
