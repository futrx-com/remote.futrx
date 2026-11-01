package web

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var assets embed.FS

func Files() fs.FS {
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return files
}
