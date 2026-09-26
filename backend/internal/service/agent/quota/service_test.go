package quota

import (
	"context"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type memoryStore struct {
	readings []AccountQuota
	saves    int
}

func (m *memoryStore) Load(context.Context) ([]AccountQuota, error) {
	return m.readings, nil
}

func (m *memoryStore) Save(_ context.Context, readings []AccountQuota) error {
	m.saves++
	m.readings = readings
	return nil
}

func percent(value float64) *float64 { return &value }

func TestRecordKeepsBothWindowsPerAccount(t *testing.T) {
	service := New(context.Background(), nil)
	service.Record(context.Background(), agent.ProviderClaude, "work", agent.Quota{
		Window:      agent.QuotaWindowSession,
		UsedPercent: percent(20),
	})
	service.Record(context.Background(), agent.ProviderClaude, "work", agent.Quota{
		Window:      agent.QuotaWindowWeekly,
		UsedPercent: percent(70),
	})

	view := service.View()
	if len(view) != 1 || view[0].Provider != "claude" || view[0].AccountID != "work" {
		t.Fatalf("expected one account, got %#v", view)
	}
	if view[0].Session == nil || *view[0].Session.UsedPercent != 20 {
		t.Fatalf("session window not kept: %#v", view[0].Session)
	}
	if view[0].Weekly == nil || *view[0].Weekly.UsedPercent != 70 {
		t.Fatalf("weekly window not kept: %#v", view[0].Weekly)
	}
}

// Two saved accounts of one provider are two plans. A reading from one must
// never replace the other's, and the host login is a third plan.
func TestRecordKeepsEachAccountsPlanApart(t *testing.T) {
	service := New(context.Background(), nil)
	for _, reading := range []struct {
		account string
		used    float64
	}{{"work", 90}, {"personal", 5}, {"", 40}} {
		service.Record(context.Background(), agent.ProviderCodex, reading.account, agent.Quota{
			Window:      agent.QuotaWindowSession,
			UsedPercent: percent(reading.used),
		})
	}

	got := map[string]float64{}
	for _, reading := range service.View() {
		got[reading.AccountID] = *reading.Session.UsedPercent
	}
	want := map[string]float64{"work": 90, "personal": 5, "": 40}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("account plans = %v; want %v", got, want)
	}
}

// A later reading of the same window replaces the earlier one; a plan card
// showing yesterday's percentage next to today's would be worse than useless.
func TestRecordReplacesTheSameWindow(t *testing.T) {
	service := New(context.Background(), nil)
	for _, used := range []float64{10, 45} {
		service.Record(context.Background(), agent.ProviderCodex, " work ", agent.Quota{
			Window:      agent.QuotaWindowSession,
			UsedPercent: percent(used),
		})
	}
	view := service.View()
	if len(view) != 1 || view[0].AccountID != "work" || *view[0].Session.UsedPercent != 45 {
		t.Fatalf("expected the latest reading, got %#v", view)
	}
}

// A window this platform does not understand is dropped rather than filed
// somewhere: guessing would make the card confidently wrong.
func TestRecordIgnoresAnUnknownWindow(t *testing.T) {
	service := New(context.Background(), nil)
	service.Record(context.Background(), agent.ProviderClaude, "", agent.Quota{Window: "monthly"})
	service.Record(context.Background(), agent.ProviderClaude, "", agent.Quota{})
	if view := service.View(); len(view) != 0 {
		t.Fatalf("expected nothing recorded, got %#v", view)
	}
}

func TestRecordIgnoresAnEmptyProvider(t *testing.T) {
	service := New(context.Background(), nil)
	service.Record(context.Background(), "  ", "work", agent.Quota{Window: agent.QuotaWindowSession})
	if view := service.View(); len(view) != 0 {
		t.Fatalf("expected nothing recorded, got %#v", view)
	}
}

func TestReadingsSurviveARestart(t *testing.T) {
	store := &memoryStore{}
	service := New(context.Background(), store)
	service.Record(context.Background(), agent.ProviderClaude, "work", agent.Quota{
		Window:      agent.QuotaWindowWeekly,
		UsedPercent: percent(80),
	})
	if store.saves == 0 {
		t.Fatal("the reading was never persisted")
	}

	restarted := New(context.Background(), store)
	view := restarted.View()
	if len(view) != 1 || view[0].AccountID != "work" || view[0].Weekly == nil || *view[0].Weekly.UsedPercent != 80 {
		t.Fatalf("reading did not survive the restart: %#v", view)
	}
}

// A persisted reading without a provider cannot be attributed to any plan.
func TestNewDropsLoadedReadingsWithoutAProvider(t *testing.T) {
	store := &memoryStore{readings: []AccountQuota{
		{Provider: " ", AccountID: "work", Session: &agent.Quota{Window: agent.QuotaWindowSession}},
		{Provider: " codex ", AccountID: " work ", Session: &agent.Quota{Window: agent.QuotaWindowSession}},
	}}
	view := New(context.Background(), store).View()
	if len(view) != 1 || view[0].Provider != "codex" || view[0].AccountID != "work" {
		t.Fatalf("loaded readings = %#v", view)
	}
}

// The card polls, so a reshuffling list would make accounts jump around
// between refreshes for no reason.
func TestViewIsStablyOrdered(t *testing.T) {
	service := New(context.Background(), nil)
	for _, reading := range []struct {
		provider agent.ProviderID
		account  string
	}{
		{agent.ProviderCodex, "b"}, {agent.ProviderClaude, "z"}, {agent.ProviderCodex, "a"}, {agent.ProviderCodex, ""},
	} {
		service.Record(context.Background(), reading.provider, reading.account, agent.Quota{Window: agent.QuotaWindowSession})
	}
	var order []string
	for _, reading := range service.View() {
		order = append(order, reading.Provider+"/"+reading.AccountID)
	}
	if want := []string{"claude/z", "codex/", "codex/a", "codex/b"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v; want %v", order, want)
	}
}

// An account nobody has run has no reading, and that is a real answer rather
// than an error or a zero.
func TestViewOfAFreshPlatformIsEmpty(t *testing.T) {
	if view := New(context.Background(), nil).View(); len(view) != 0 {
		t.Fatalf("expected no readings, got %#v", view)
	}
}
