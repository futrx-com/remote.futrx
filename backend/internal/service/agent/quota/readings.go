package quota

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// readErrorLimit bounds the provider error text kept for one account.
const readErrorLimit = 300

// accountReadings owns the durable quota snapshot and the transient errors
// that explain failed live reads. It serializes each mutation through its
// synchronous save so an older snapshot is never persisted after a newer one.
type accountReadings struct {
	writeMu sync.Mutex
	mu      sync.RWMutex
	values  map[accountKey]AccountQuota
	errors  map[accountKey]string
	store   Repository
}

func newAccountReadings(ctx context.Context, store Repository) *accountReadings {
	readings := &accountReadings{
		values: make(map[accountKey]AccountQuota),
		errors: make(map[accountKey]string),
		store:  store,
	}
	if store == nil {
		return readings
	}
	loaded, err := store.Load(ctx)
	if err != nil {
		return readings
	}
	for _, reading := range loaded {
		reading.Provider = strings.TrimSpace(reading.Provider)
		reading.AccountID = strings.TrimSpace(reading.AccountID)
		if reading.Provider == "" {
			continue
		}
		readings.values[reading.key()] = reading.clone()
	}
	return readings
}

func (r *accountReadings) record(
	ctx context.Context,
	provider agent.ProviderID,
	accountID string,
	quota agent.Quota,
) {
	key := newAccountKey(provider, accountID)
	if key.provider == "" {
		return
	}
	if quota.Window != agent.QuotaWindowSession && quota.Window != agent.QuotaWindowWeekly {
		return
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	r.mu.Lock()
	current := r.values[key]
	current.Provider = key.provider
	current.AccountID = key.account
	switch quota.Window {
	case agent.QuotaWindowSession:
		current.Session = cloneWindow(&quota)
	case agent.QuotaWindowWeekly:
		current.Weekly = cloneWindow(&quota)
	}
	r.values[key] = current
	r.saveLocked(ctx)
}

// replace files one account's live answer. Its windows replace the last
// reading, because the vendor has just said which windows are in effect. A
// failed read keeps the last windows and records why.
func (r *accountReadings) replace(
	ctx context.Context,
	provider agent.ProviderID,
	usage agent.AccountPlanUsage,
) {
	key := newAccountKey(provider, usage.AccountID)
	if key.provider == "" {
		return
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	r.mu.Lock()
	if usage.Err != nil {
		r.errors[key] = readErrorText(usage.Err)
		r.mu.Unlock()
		return
	}
	delete(r.errors, key)
	reading := AccountQuota{Provider: key.provider, AccountID: key.account}
	for _, window := range usage.Windows {
		switch window.Window {
		case agent.QuotaWindowSession:
			reading.Session = cloneWindow(&window)
		case agent.QuotaWindowWeekly:
			reading.Weekly = cloneWindow(&window)
		}
	}
	if reading.Session == nil && reading.Weekly == nil {
		delete(r.values, key)
	} else {
		r.values[key] = reading
	}
	r.saveLocked(ctx)
}

// saveLocked releases r.mu, then persists a snapshot taken while it was held.
// r.mu and r.writeMu must be held by the caller.
func (r *accountReadings) saveLocked(ctx context.Context) {
	if r.store == nil {
		r.mu.Unlock()
		return
	}
	snapshot := r.snapshotLocked()
	r.mu.Unlock()
	_ = r.store.Save(ctx, snapshot)
}

func (r *accountReadings) view() []AccountView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]AccountView, 0, len(r.values)+len(r.errors))
	for key, reading := range r.values {
		out = append(out, AccountView{AccountQuota: reading.clone(), Error: r.errors[key]})
	}
	for key, message := range r.errors {
		if _, ok := r.values[key]; !ok {
			out = append(out, AccountView{
				AccountQuota: AccountQuota{Provider: key.provider, AccountID: key.account},
				Error:        message,
			})
		}
	}
	sortAccountViews(out)
	return out
}

// snapshotLocked copies every reading, so the repository never shares windows
// with the in-memory owner. r.mu must be held by the caller.
func (r *accountReadings) snapshotLocked() []AccountQuota {
	out := make([]AccountQuota, 0, len(r.values))
	for _, reading := range r.values {
		out = append(out, reading.clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

func sortAccountViews(views []AccountView) {
	sort.Slice(views, func(i, j int) bool {
		if views[i].Provider != views[j].Provider {
			return views[i].Provider < views[j].Provider
		}
		return views[i].AccountID < views[j].AccountID
	})
}

func newAccountKey(provider agent.ProviderID, accountID string) accountKey {
	return accountKey{
		provider: strings.TrimSpace(string(provider)),
		account:  strings.TrimSpace(accountID),
	}
}

// readErrorText keeps a provider error to one bounded line.
func readErrorText(err error) string {
	message := strings.Join(strings.Fields(err.Error()), " ")
	if runes := []rune(message); len(runes) > readErrorLimit {
		message = string(runes[:readErrorLimit]) + "…"
	}
	return message
}
