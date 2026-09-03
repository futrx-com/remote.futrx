// build-base-image rebuilds the futrx-remote-dev-base LXD image used by
// every project container. Run it after changing Node, any configured agent
// profile or CLI, or an apt dependency in the install script.
//
// Usage:
//
//	go run ./cmd/build-base-image                 # build into the default alias
//	go run ./cmd/build-base-image -overwrite      # delete the existing alias first
//	go run ./cmd/build-base-image -alias mybase   # publish under a custom alias
//
// This binary is self-contained: it requires only the `lxc` CLI on PATH and
// network access to apt + npm. Re-running it on a fresh host is the one-shot
// bootstrap for the application's container fleet.
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/config"
	"github.com/futrx-com/remote.futrx.com/internal/integration/lxc"
	serviceimage "github.com/futrx-com/remote.futrx.com/internal/service/container/image"
)

func main() {
	alias := flag.String("alias", serviceimage.Alias, "image alias to publish under")
	overwrite := flag.Bool("overwrite", false, "delete any existing image at -alias before publishing")
	flag.Parse()

	log.SetFlags(log.Ltime)

	lxcClient := lxc.New()
	if !lxcClient.Available() {
		log.Fatalf("lxc CLI not found on PATH - install LXD on the host first")
	}
	agentModules, err := config.NewAgentModules()
	if err != nil {
		log.Fatalf("configure agent modules: %v", err)
	}
	containerStack := config.NewContainerStack(
		lxcClient,
		agentModules.Profiles(),
		config.ContainerStackOptions{
			ImageBuildProgress: newLogBuildProgressReporter(log.Default()),
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	if *overwrite {
		log.Printf("removing existing image %q (if any)...", *alias)
		// Best-effort: ignore the error so a missing alias is fine.
		if out, err := lxcClient.Run(ctx, "image", "delete", *alias); err != nil {
			log.Printf("note: image delete returned: %v; output: %s", err, out)
		}
	}

	log.Printf("building %q from %q...", *alias, serviceimage.SourceImage)
	log.Printf("(the first build can take up to 10 minutes; progress is reported every 30 seconds)")

	if err := containerStack.Images.Build(ctx, *alias); err != nil {
		log.Fatalf("build failed: %v", err)
	}

	log.Printf("done. published %q. new project containers will launch from this image.", *alias)
}
