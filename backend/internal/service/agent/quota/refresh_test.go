package quota

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// fakeReader answers every read with the answer the test set, optionally
// holding each read until the test releases it.
type fakeReader struct {
	provider agent.ProviderID
	mu       sync.Mutex
	answer   []agent.AccountPlanUsage
	reads    atomic.Int32
	started  chan struct{}
	release  chan struct{}
}

func (r *fakeReader) ID() agent.ProviderID { return r.provider }

func (r *fakeReader) ReadPlanUsage(ctx context.Context) []agent.AccountPlanUsage {
	r.reads.Add(1)
	if r.started != nil {
		r.started <- struct{}{}
	}
	if r.release != nil {
		<-r.release
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.answer
}

func (r *fakeReader) set(answer ...agent.AccountPlanUsage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.answer = answer
}

func usedWindow(window agent.QuotaWindow, used float64) agent.Quota {
	return agent.Quota{Window: window, UsedPercent: percent(used), MeasuredAt: 1}
}

// windowsOf summarizes the view as provider/account → window=percent.
func windowsOf(view []AccountView) map[string][]string {
	out := map[string][]string{}
	for _, account := range view {
		key := account.Provider + "/" + account.AccountID
		out[key] = []string{}
		for _, window := range []*agent.Quota{account.Session, account.Weekly} {
			if window != nil {
				out[key] = append(out[key], string(window.Window)+"="+strconv.FormatFloat(*window.UsedPercent, 'f', -1, 64))
			}
		}
		if account.Error != "" {
			out[key] = append(out[key], "error="+account.Error)
		}
	}
	return out
}

func TestRefreshReadsEveryAccountAndReplacesItsWindows(t *testing.T) {
	store := &memoryStore{}
	codex := &fakeReader{provider: agent.ProviderCodex}
	codex.set(
		agent.AccountPlanUsage{AccountID: "work", Windows: []agent.Quota{
			usedWindow(agent.QuotaWindowSession, 40), usedWindow(agent.QuotaWindowWeekly, 12),
		}},
		agent.AccountPlanUsage{AccountID: "personal", Windows: []agent.Quota{usedWindow(agent.QuotaWindowWeekly, 70)}},
	)
	service := New(context.Background(), store, codex)
	now := time.Unix(1_000, 0)
	service.refresher.now = func() time.Time { return now }

	service.Refresh(context.Background())
	want := map[string][]string{
		"codex/personal": {"weekly=70"},
		"codex/work":     {"session=40", "weekly=12"},
	}
	if got := windowsOf(service.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("view = %v; want %v", got, want)
	}
	if len(store.readings) != 2 {
		t.Fatalf("persisted readings = %+v", store.readings)
	}

	// A live answer is the whole plan: a window it no longer reports is no
	// longer in effect, and an account without windows has nothing to show.
	codex.set(
		agent.AccountPlanUsage{AccountID: "work", Windows: []agent.Quota{usedWindow(agent.QuotaWindowSession, 55)}},
		agent.AccountPlanUsage{AccountID: "personal"},
	)
	now = now.Add(service.refresher.refreshInterval)
	service.Refresh(context.Background())
	want = map[string][]string{"codex/work": {"session=55"}}
	if got := windowsOf(service.View()); !reflect.DeepEqual(got, want) {
		t.Fatalf("view after second read = %v; want %v", got, want)
	}
}

func TestRefreshSharesOneAnswerWithinTheInterval(t *testing.T) {
	codex := &fakeReader{provider: agent.ProviderCodex}
	service := New(context.Background(), nil, codex)
	now := time.Unix(1_000, 0)
	service.refresher.now = func() time.Time { return now }

	service.Refresh(context.Background())
	now = now.Add(service.refresher.refreshInterval - time.Second)
	service.Refresh(context.Background())
	if reads := codex.reads.Load(); reads != 1 {
		t.Fatalf("reads within the interval = %d; want 1", reads)
	}
	now = now.Add(time.Second)
	service.Refresh(context.Background())
	if reads := codex.reads.Load(); reads != 2 {
		t.Fatalf("reads after the interval = %d; want 2", reads)
	}
}

func TestRefreshSharesAReadInProgress(t *testing.T) {
	codex := &fakeReader{
		provider: agent.ProviderCodex, started: make(chan struct{}, 2), release: make(chan struct{}),
	}
	codex.set(agent.AccountPlanUsage{AccountID: "work", Windows: []agent.Quota{usedWindow(agent.QuotaWindowSession, 40)}})
	service := New(context.Background(), nil, codex)

	var callers sync.WaitGroup
	for range 3 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			service.Refresh(context.Background())
		}()
	}
	<-codex.started
	close(codex.release)
	callers.Wait()
	if reads := codex.reads.Load(); reads != 1 {
		t.Fatalf("reads = %d; want one read shared by every caller", reads)
	}
	if got := windowsOf(service.View()); !reflect.DeepEqual(got, map[string][]string{"codex/work": {"session=40"}}) {
		t.Fatalf("view = %v", got)
	}
}

// A caller that stops waiting must not cancel the read everyone else shares:
// the answer still lands for the next request.
func TestRefreshOutlivesACallerThatStopsWaiting(t *testing.T) {
	codex := &fakeReader{
		provider: agent.ProviderCodex, started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	codex.set(agent.AccountPlanUsage{AccountID: "work", Windows: []agent.Quota{usedWindow(agent.QuotaWindowWeekly, 30)}})
	service := New(context.Background(), nil, codex)

	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})
	go func() {
		service.Refresh(ctx)
		close(returned)
	}()
	<-codex.started
	cancel()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh kept a caller whose context ended")
	}
	close(codex.release)

	deadline := time.After(5 * time.Second)
	for len(service.View()) == 0 {
		select {
		case <-deadline:
			t.Fatal("the shared read never recorded its answer")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A failed read explains itself without throwing away the last good windows,
// and the next good read clears the explanation.
func TestRefreshKeepsWindowsThroughAFailedRead(t *testing.T) {
	claude := &fakeReader{provider: agent.ProviderClaude}
	service := New(context.Background(), nil, claude)
	now := time.Unix(1_000, 0)
	service.refresher.now = func() time.Time { return now }
	read := func(answer ...agent.AccountPlanUsage) map[string][]string {
		t.Helper()
		claude.set(answer...)
		now = now.Add(service.refresher.refreshInterval)
		service.Refresh(context.Background())
		return windowsOf(service.View())
	}

	good := agent.AccountPlanUsage{AccountID: "work", Windows: []agent.Quota{usedWindow(agent.QuotaWindowSession, 20)}}
	read(good)
	failed := agent.AccountPlanUsage{AccountID: "work", Err: errors.New("sign-in\n  expired")}
	if got, want := read(failed), map[string][]string{"claude/work": {"session=20", "error=sign-in expired"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after failed read = %v; want %v", got, want)
	}
	unread := agent.AccountPlanUsage{AccountID: "new", Err: errors.New("offline")}
	if got, want := read(good, unread), map[string][]string{
		"claude/new":  {"error=offline"},
		"claude/work": {"session=20"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after recovery = %v; want %v", got, want)
	}
}

func TestRefreshWithoutReadersReturnsAtOnce(t *testing.T) {
	service := New(context.Background(), nil)
	service.Refresh(context.Background())
	if view := service.View(); len(view) != 0 {
		t.Fatalf("view = %+v", view)
	}
}
