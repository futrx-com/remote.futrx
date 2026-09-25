package main

import (
	"encoding/json"
	"fmt"
	"os"

	"futrx.local/catalog/applications/hello-remote/backend/container/internal/containerinfo"
)

var version = "development"

// REQUIRED — each cmd/<name> program needs package main and func main().
// Remote supplies packaging, the Go toolchain, installation, and build markers.
func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: hello-remote-info [--version]")
		os.Exit(2)
	}
	info, err := containerinfo.Read()
	if err != nil {
		fmt.Fprintf(os.Stderr, "inspect container: %v\n", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
		fmt.Fprintf(os.Stderr, "encode container info: %v\n", err)
		os.Exit(1)
	}
}
