package usage

import (
	"context"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// Repository is the ledger's persistence port. Implementations own the
// monthly file layout; the service only asks for records in a time window.
type Repository interface {
	// Append adds one record to the file for its month.
	Append(ctx context.Context, record Record) error
	// Scan visits every stored record whose timestamp falls in [from, to],
	// oldest month first. A non-positive bound is unbounded on that side, so
	// Scan(ctx, 0, 0, ...) walks the whole ledger. A false return from visit
	// stops the scan.
	Scan(ctx context.Context, from, to int64, visit func(Record) bool) error
	// ReplaceAll rewrites the ledger from scratch and reports the month keys
	// it wrote. Months with no records are removed.
	ReplaceAll(ctx context.Context, records []Record) ([]string, error)
	// Prices reads the editable price table, seeding it on first use.
	Prices(ctx context.Context) (PriceTable, error)
	// SetPrices replaces the editable price table.
	SetPrices(ctx context.Context, table PriceTable) (PriceTable, error)
}

// ProjectDirectory resolves project names and per-caller visibility.
type ProjectDirectory interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	ListVisible(ctx context.Context, email string, isAdmin bool) ([]serviceproject.Meta, error)
}

// ChatDirectory is what a rebuild reads: every chat's metadata and its
// persisted event log.
type ChatDirectory interface {
	List(ctx context.Context) ([]servicechat.Meta, error)
	ReadEvents(ctx context.Context, id servicechat.ID) ([]servicechat.Event, error)
}
