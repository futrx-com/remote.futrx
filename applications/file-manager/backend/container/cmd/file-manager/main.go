package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/presentation/web"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/archives"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/workspace"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/service/files"
	httptransport "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http"
	httphandlers "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http/handlers"
	httpmiddleware "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http/middleware"
)

func main() {
	port := flag.Int("port", config.DefaultPort, "HTTP port")
	flag.Parse()
	if *port < 1 || *port > 65535 {
		log.Fatal("invalid port")
	}
	spool := archives.New(config.MaxConcurrentArchives, config.MaxArchiveSpoolBytes)
	service := files.New(config.WorkspaceDirectory, workspace.New(), spool)
	handler := httptransport.NewHandler(httptransport.Handlers{
		Files:      httphandlers.NewFileHandler(service),
		Static:     http.FileServer(http.FS(web.Files())),
		Middleware: httpmiddleware.PageHeaders{},
	})
	server := httptransport.NewServer("0.0.0.0:"+strconv.Itoa(*port), handler)
	log.Fatal(server.ListenAndServe())
}
