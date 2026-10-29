package application

import "errors"

var (
	ErrInvalidPath     = errors.New("invalid path")
	ErrFileNotFound    = errors.New("file not found")
	ErrFolderNotFound  = errors.New("folder not found")
	ErrArchiveTooLarge = errors.New("workspace archive exceeds download size limit")
)
