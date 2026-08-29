package agentquota

import (
	"context"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type memoryStore struct {
	readings map[string]AgentQuota
	saves    int
}

func (m *memoryStore) Load(context.Context) (map[string]AgentQuota, error) {
	return m.readings, nil
}

func (m *memoryStore) Save(_ context.Context, readings map[string]AgentQuota) error {
	m.saves++
	m.readings = readings
	return nil
}

func percent(value float64) *float64 { return &value }

func TestRecordKeepsBothWindowsPerAgent(t *testing.T) {
	service := New(context.Background(), nil)
	service.Record(context.Background(), agent.ProviderClaude, agent.Quota{
		Window:      agent.QuotaWindowSession,
		UsedPercent: percent(20),
	})
	service.Record(context.Background(), agent.ProviderClaude, agent.Quota{
		Window:      agent.QuotaWindowWeekly,
		UsedPercent: percent(70),
	})

	view := service.View()
	if len(view) != 1 {
		t.Fatalf("expected one agent, got %#v", view)
	}
	if view[0].Session == nil || *view[0].Session.UsedPercent != 20 {
		t.Fatalf("session window not kept: %#v", view[0].Session)
	}
	if view[0].Weekly == nil || *view[0].Weekly.UsedPercent != 70 {
		t.Fatalf("weekly window not kept: %#v", view[0].Weekly)
	}
}

// A later reading of the same window replaces the earlier one; a plan card
// showing yesterday's percentage next to today's would be worse than useless.
func TestRecordReplacesTheSameWindow(t *testing.T) {
	service := New(context.Background(), nil)
	for _, used := range []float64{10, 45} {
		service.Record(context.Background(), agent.ProviderCodex, agent.Quota{
			Window:      agent.QuotaWindowSession,
			UsedPercent: percent(used),
		})
	}
	view := service.View()
	if len(view) != 1 || *view[0].Session.UsedPercent != 45 {
		t.Fatalf("expected the latest reading, got %#v", view)
	}
}

// A window this platform does not understand is dropped rather than filed
// somewhere: guessing would make the card confidently wrong.
func TestRecordIgnoresAnUnknownWindow(t *testing.T) {
	service := New(context.Background(), nil)
	service.Record(context.Background(), agent.ProviderClaude, agent.Quota{Window: "monthly"})
	service.Record(context.Background(), agent.ProviderClaude, agent.Quota{})
	if view := service.View(); len(view) != 0 {
		t.Fatalf("expected nothing recorded, got %#v", view)
	}
}

func TestRecordIgnoresAnEmptyProvider(t *testing.T) {
	service := New(context.Background(), nil)
	service.Record(context.Background(), "  ", agent.Quota{Window: agent.QuotaWindowSession})
	if view := service.View(); len(view) != 0 {
		t.Fatalf("expected nothing recorded, got %#v", view)
	}
}

func TestReadingsSurviveARestart(t *testing.T) {
	store := &memoryStore{}
	service := New(context.Background(), store)
	service.Record(context.Background(), agent.ProviderClaude, agent.Quota{
		Window:      agent.QuotaWindowWeekly,
		UsedPercent: percent(80),
	})
	if store.saves == 0 {
		t.Fatal("the reading was never persisted")
	}

	restarted := New(context.Background(), store)
	view := restarted.View()
	if len(view) != 1 || view[0].Weekly == nil || *view[0].Weekly.UsedPercent != 80 {
		t.Fatalf("reading did not survive the restart: %#v", view)
	}
}

// The card polls, so a reshuffling list would make agents jump around between
// refreshes for no reason.
func TestViewIsStablyOrdered(t *testing.T) {
	service := New(context.Background(), nil)
	for _, provider := range []agent.ProviderID{agent.ProviderCodex, agent.ProviderClaude} {
		service.Record(context.Background(), provider, agent.Quota{Window: agent.QuotaWindowSession})
	}
	view := service.View()
	if len(view) != 2 || view[0].Provider != string(agent.ProviderClaude) {
		t.Fatalf("unexpected order: %#v", view)
	}
}

// An agent nobody has run has no reading, and that is a real answer rather
// than an error or a zero.
func TestViewOfAFreshPlatformIsEmpty(t *testing.T) {
	if view := New(context.Background(), nil).View(); len(view) != 0 {
		t.Fatalf("expected no readings, got %#v", view)
	}
}
