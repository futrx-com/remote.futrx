package workspacefiles

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/shared/workspacepath"
)

var (
	ErrFileNotFound     = errors.New("file not found")
	ErrUnsupportedMedia = errors.New("file type cannot be opened in browser")
)

// Store is the filesystem boundary retained for chat-message media links. The
// File Management application owns browsing, search, and downloads.
type Store interface {
	OpenFile(root, relative string) (io.ReadSeekCloser, string, time.Time, error)
}

type Service struct {
	store Store
}

func New(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) OpenMedia(cwd, rawPath string) (Media, error) {
	target, err := workspacepath.ResolveFile(rawPath, cwd)
	if err != nil {
		return Media{}, err
	}
	contentType, supported := supportedMediaType(target.FilePath)
	if !supported {
		return Media{}, ErrUnsupportedMedia
	}
	relative, err := filepath.Rel(target.WorkspaceRoot, target.FilePath)
	if err != nil {
		return Media{}, ErrFileNotFound
	}
	content, _, modTime, err := s.store.OpenFile(target.WorkspaceRoot, filepath.ToSlash(relative))
	if err != nil {
		return Media{}, ErrFileNotFound
	}
	return Media{
		File: &File{
			Name:    filepath.Base(target.FilePath),
			ModTime: modTime,
			content: content,
		},
		ContentType: contentType,
	}, nil
}

func supportedMediaType(path string) (string, bool) {
	contentType, ok := mediaTypes[strings.ToLower(filepath.Ext(path))]
	return contentType, ok
}

var mediaTypes = map[string]string{
	".aac":  "audio/aac",
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".flac": "audio/flac",
	".gif":  "image/gif",
	".ico":  "image/x-icon",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".m4a":  "audio/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".mp3":  "audio/mpeg",
	".mp4":  "video/mp4",
	".oga":  "audio/ogg",
	".ogg":  "audio/ogg",
	".ogv":  "video/ogg",
	".opus": "audio/opus",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".wav":  "audio/wav",
	".webm": "video/webm",
	".webp": "image/webp",
}
