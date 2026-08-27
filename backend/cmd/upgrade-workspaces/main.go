// upgrade-workspaces replaces project containers through the application's Go
// lifecycle. It is invoked after the new base image is published; shell code
// never deletes LXC instances directly.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/config"
	"github.com/futrx-com/remote.futrx.com/internal/integration/lxc"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "show the project upgrade plan without changing containers")
	includeBusy := flag.Bool("include-busy", false, "stop and replace containers with active agent processes")
	flag.Parse()

	cfg := config.Load()
	storeSet, err := stores.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("init stores: %v", err)
	}
	lxcClient := lxc.New()
	agentModules, err := config.NewAgentModules()
	if err != nil {
		log.Fatalf("configure agent modules: %v", err)
	}
	containerStack := config.NewContainerStack(lxcClient, agentModules.Profiles(), config.ContainerStackOptions{})
	projects := serviceproject.New(
		storeSet.Projects,
		containerStack.ProjectDependencies(),
		storeSet.ProjectSecrets,
		storeSet.ProjectAccess,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	metas, err := projects.List(ctx)
	if err != nil {
		log.Fatalf("list projects: %v", err)
	}

	upgraded, skipped, failed := 0, 0, 0
	for _, meta := range metas {
		state, stateErr := containerStack.Lifecycle.State(ctx, meta.ContainerName)
		if stateErr != nil {
			failed++
			log.Printf("FAIL %s: inspect container: %v", meta.Slug, stateErr)
			continue
		}
		busy := false
		if state != serviceproject.ContainerStateMissing {
			busy, err = containerStack.Lifecycle.Busy(ctx, meta.ContainerName)
			if err != nil {
				failed++
				log.Printf("FAIL %s: inspect activity: %v", meta.Slug, err)
				continue
			}
		}
		if busy && !*includeBusy {
			skipped++
			log.Printf("SKIP %s: active agent process", meta.Slug)
			continue
		}
		if *dryRun {
			upgraded++
			action := "replace"
			if state == serviceproject.ContainerStateMissing {
				action = "create"
			}
			log.Printf("PLAN %s: %s and validate container", meta.Slug, action)
			continue
		}

		if _, err := projects.Upgrade(ctx, meta.ID, *includeBusy); err != nil {
			if errors.Is(err, serviceproject.ErrProjectBusy) {
				skipped++
				log.Printf("SKIP %s: active agent process", meta.Slug)
				continue
			}
			failed++
			log.Printf("FAIL %s: %v", meta.Slug, err)
			continue
		}
		upgraded++
		log.Printf("OK %s: persistent agent state migrated; container replaced and validated", meta.Slug)
	}

	fmt.Printf("workspace upgrade: %d upgraded, %d busy skipped, %d failed\n", upgraded, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
