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
	servicelifecycle "github.com/futrx-com/remote.futrx.com/internal/service/container/lifecycle"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "show the project upgrade plan without changing containers")
	includeBusy := flag.Bool("include-busy", false, "stop and replace containers with active agent processes")
	progressFile := flag.String("progress-file", "", "atomically publish structured workspace-upgrade progress")
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

	progress := newProgressWriter(*progressFile)
	upgraded, skipped, failed, err := upgradeAll(
		ctx,
		projects,
		containerStack.Lifecycle,
		*dryRun,
		*includeBusy,
		progress,
	)
	if err != nil {
		log.Fatalf("list projects: %v", err)
	}

	fmt.Printf("workspace upgrade: %d upgraded, %d busy skipped, %d failed\n", upgraded, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// upgradeAll walks every project and replaces its container onto the current
// base image, applying the dry-run and include-busy policy per project. It
// logs one outcome line per project as it goes and returns the tallies main
// reports.
func upgradeAll(
	ctx context.Context,
	projects *serviceproject.Service,
	lifecycle *servicelifecycle.Service,
	dryRun, includeBusy bool,
	progress progressWriter,
) (upgraded, skipped, failed int, err error) {
	metas, err := projects.List(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	total := len(metas)
	if err := progress.write(0, total, "", "Preparing workspace migration"); err != nil {
		log.Printf("progress: %v", err)
	}

	for index, meta := range metas {
		message := fmt.Sprintf("Recycling workspace %d of %d", index+1, total)
		if err := progress.write(index, total, meta.Slug, message); err != nil {
			log.Printf("progress: %v", err)
		}
		state, stateErr := lifecycle.State(ctx, meta.ContainerName)
		if stateErr != nil {
			failed++
			log.Printf("FAIL %s: inspect container: %v", meta.Slug, stateErr)
			continue
		}
		busy := false
		if state != serviceproject.ContainerStateMissing {
			busy, err = lifecycle.Busy(ctx, meta.ContainerName)
			if err != nil {
				failed++
				log.Printf("FAIL %s: inspect activity: %v", meta.Slug, err)
				continue
			}
		}
		if busy && !includeBusy {
			skipped++
			log.Printf("SKIP %s: active agent process", meta.Slug)
			continue
		}
		if dryRun {
			upgraded++
			action := "replace"
			if state == serviceproject.ContainerStateMissing {
				action = "create"
			}
			log.Printf("PLAN %s: %s and validate container", meta.Slug, action)
			continue
		}

		if _, err := projects.Upgrade(ctx, meta.ID, includeBusy); err != nil {
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
		if err := progress.write(index+1, total, "", message); err != nil {
			log.Printf("progress: %v", err)
		}
	}

	summary := fmt.Sprintf("Workspace migration complete: %d upgraded, %d busy skipped, %d failed", upgraded, skipped, failed)
	if err := progress.write(total, total, "", summary); err != nil {
		log.Printf("progress: %v", err)
	}
	return upgraded, skipped, failed, nil
}
