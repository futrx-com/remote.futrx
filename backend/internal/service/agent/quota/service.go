// Package quota remembers the last subscription-quota reading each provider
// account reports, so the dashboard can show how much of each plan is left.
//
// Readings arrive two ways. Runs report the windows the CLIs mention while
// they work, and Refresh asks every provider that can read plan limits on
// demand, the way Claude Code's /usage and Codex's /status do. Three
// consequences shape this service:
//
//   - A reading is a snapshot, not a live figure. Every reading carries when
//     it was taken and the UI is expected to say so; a stale number presented
//     as current is worse than an empty card, because an operator would plan
//     around it.
//   - An account nobody has read has no reading at all, and that is not an
//     error. It is the honest state, and it is different from "0% used".
//   - A plan belongs to a provider account, not to the provider. Claude and
//     Codex keep several saved accounts and a chat may pin any of them, so
//     each account's windows are kept apart. Which saved accounts still exist
//     is the account service's answer; readings are only keyed by account ID.
//
// It is also not the usage ledger. The ledger knows what this platform spent;
// the plan is spent from everywhere the operator works. Only the vendor knows
// the total, and these readings are the vendor talking.
package quota

import (
	"context"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// Repository persists readings across restarts.
type Repository interface {
	Load(ctx context.Context) ([]AccountQuota, error)
	Save(ctx context.Context, readings []AccountQuota) error
}

// Service composes the durable account readings with their live refresh
// lifecycle. Each collaborator owns its own synchronization.
type Service struct {
	readings  *accountReadings
	refresher *planUsageRefresher
}

// New loads saved readings from store and asks readers for live plan limits
// whenever Refresh finds the last answer out of date.
func New(ctx context.Context, store Repository, readers ...agent.PlanUsageReader) *Service {
	readings := newAccountReadings(ctx, store)
	return &Service{
		readings: readings,
		refresher: newPlanUsageRefresher(
			readings,
			readers,
			configconstants.PlanUsageRefreshInterval,
			configconstants.PlanUsageReadTimeout,
			time.Now,
		),
	}
}

// Record files one reading against the provider account that reported it. An
// empty accountID stands for the provider's host login.
//
// Persisting is synchronous but best effort. Losing the last reading of a
// window costs a stale dashboard until the next observation, while failing a
// turn over it would cost the operator their actual work.
func (s *Service) Record(ctx context.Context, provider agent.ProviderID, accountID string, quota agent.Quota) {
	if s == nil {
		return
	}
	s.readings.record(ctx, provider, accountID, quota)
}

// Refresh asks every provider that can read plan limits on demand for each
// account's current windows. An answer stays current for the refresh
// interval, and callers within it, or while a read is running, share it.
// The read belongs to no single caller: it continues, bounded by the read
// timeout, after a caller whose ctx ends stops waiting for it.
func (s *Service) Refresh(ctx context.Context) {
	if s == nil {
		return
	}
	s.refresher.refresh(ctx)
}

// View lists every account with a reading or a failed read, by provider and
// then account, in a stable order so the card does not reshuffle between
// polls.
func (s *Service) View() []AccountView {
	if s == nil {
		return nil
	}
	return s.readings.view()
}
