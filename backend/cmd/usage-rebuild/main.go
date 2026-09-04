// usage-rebuild regenerates DATA_DIR/usage/usage-YYYY-MM.jsonl from the
// persisted chat event logs.
//
// It is the offline twin of POST /api/admin/usage/rebuild and exists for
// installs that upgraded into the usage feature with years of chat history:
// run it once with the service stopped, then start the service again.
//
//	usage-rebuild                       # uses $DATA_DIR
//	usage-rebuild -data-dir /opt/remote.futrx/data
//	usage-rebuild -dry-run              # report only, write nothing
//
// The rebuild is idempotent, so running it twice is harmless.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusage"
)

func main() {
	defaultDataDir := os.Getenv("DATA_DIR")
	if defaultDataDir == "" {
		defaultDataDir = "/opt/remote.futrx/data"
	}
	dataDir := flag.String("data-dir", defaultDataDir, "Remote data directory (DATA_DIR)")
	dryRun := flag.Bool("dry-run", false, "report what would be written without touching the ledger")
	flag.Parse()

	if err := run(context.Background(), *dataDir, *dryRun); err != nil {
		log.Fatalf("usage-rebuild: %v", err)
	}
}

func run(ctx context.Context, dataDir string, dryRun bool) error {
	chats, err := filechat.New(dataDir)
	if err != nil {
		return fmt.Errorf("open chat store: %w", err)
	}
	projects, err := fileproject.New(dataDir)
	if err != nil {
		return fmt.Errorf("open project store: %w", err)
	}
	ledger, err := fileusage.New(dataDir)
	if err != nil {
		return fmt.Errorf("open usage store: %w", err)
	}

	var repository serviceusage.Repository = ledger
	if dryRun {
		repository = readOnlyRepository{Repository: ledger}
	}
	service := serviceusage.New(repository, projectDirectory{store: projects}, chats)

	result, err := service.Rebuild(ctx)
	if err != nil {
		return err
	}
	verb := "wrote"
	if dryRun {
		verb = "would write"
	}
	fmt.Printf(
		"scanned %d chats, %s %d usage records across %d month(s)%s; preserved %d user attributions\n",
		result.Chats,
		verb,
		result.Records,
		len(result.Months),
		formatMonths(result.Months),
		result.PreservedActors,
	)
	return nil
}

func formatMonths(months []string) string {
	if len(months) == 0 {
		return ""
	}
	return " (" + strings.Join(months, ", ") + ")"
}

// projectDirectory adapts the plain project store to the ledger's directory
// port. Offline there is no caller, so every project is visible.
type projectDirectory struct {
	store serviceproject.Repository
}

func (d projectDirectory) Get(
	ctx context.Context,
	id serviceproject.ID,
) (serviceproject.Meta, error) {
	return d.store.Get(ctx, id)
}

func (d projectDirectory) ListVisible(
	ctx context.Context,
	_ string,
	_ bool,
) ([]serviceproject.Meta, error) {
	return d.store.List(ctx)
}

// readOnlyRepository makes -dry-run genuinely read-only: the rebuild still
// computes every record, but the write back to disk is dropped.
type readOnlyRepository struct {
	serviceusage.Repository
}

func (r readOnlyRepository) ReplaceAll(
	_ context.Context,
	records []serviceusage.Record,
) ([]string, error) {
	seen := map[string]struct{}{}
	months := make([]string, 0, 2)
	for _, record := range records {
		month := serviceusage.MonthKey(record.At)
		if _, ok := seen[month]; ok {
			continue
		}
		seen[month] = struct{}{}
		months = append(months, month)
	}
	sort.Strings(months)
	return months, nil
}
