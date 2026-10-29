package inbound

import (
	"context"

	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

type Files interface {
	List(relativePath string) (model.Listing, error)
	Search(query string) (model.SearchResult, error)
	OpenFile(relativePath string) (*model.File, error)
	DownloadArchive(context.Context, string) (*model.File, error)
}
