package prompt

import (
	"context"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

type recordedQuota struct {
	ctx       context.Context
	provider  harness.ProviderID
	accountID string
	quota     harness.Quota
}

type recordingQuota struct {
	readings []recordedQuota
}

func (r *recordingQuota) Record(ctx context.Context, provider harness.ProviderID, accountID string, quota harness.Quota) {
	r.readings = append(r.readings, recordedQuota{ctx: ctx, provider: provider, accountID: accountID, quota: quota})
}

// quotaProvider reports one plan window the way a saved-account run does: the
// adapter stamps the account and leaves the provider for the relay to fill in.
type quotaProvider struct {
	usageProvider
	accountID string
	reading   harness.Quota
}

func (p *quotaProvider) Run(ctx context.Context, req harness.RunRequest, emit func(harness.Event)) error {
	reading := p.reading
	emit(harness.Event{T: time.Now().UnixMilli(), Type: harness.EventQuotaUpdated, AccountID: p.accountID, Quota: &reading})
	return p.usageProvider.Run(ctx, req, emit)
}

func TestStartFilesPlanLimitsUnderTheAccountThatRan(t *testing.T) {
	recorder := &recordingQuota{}
	reading := harness.Quota{Window: harness.QuotaWindowSession, Status: "allowed", MeasuredAt: 123}
	provider := &quotaProvider{accountID: "work", reading: reading}
	service, _, meta := newUsagePromptService(t, provider, &recordingLedger{}, WithQuotaRecorder(recorder))

	handle, err := service.Start(StartInput{
		ChatID:        meta.ID,
		Prompt:        "hello",
		Actor:         Actor{Email: "member@example.com"},
		ParentContext: context.Background(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-handle.Done

	if len(recorder.readings) != 1 {
		t.Fatalf("recorded = %+v", recorder.readings)
	}
	got := recorder.readings[0]
	if got.provider != harness.ProviderClaude || got.accountID != "work" || got.quota != reading {
		t.Fatalf("recorded = %+v; want claude/work %+v", got, reading)
	}
}

func TestRecordQuotaPreservesValuesWithoutCancellation(t *testing.T) {
	type contextKey struct{}
	live, cancelLive := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "run"))
	defer cancelLive()
	cancelled, cancel := context.WithCancel(live)
	cancel()
	for _, ctx := range []context.Context{live, cancelled} {
		recorder := &recordingQuota{}
		service := &Service{quota: recorder}
		reading := harness.Quota{Window: harness.QuotaWindowSession, MeasuredAt: 123}
		service.recordQuota(ctx, harness.Event{
			Type: harness.EventQuotaUpdated, Provider: harness.ProviderCodex, AccountID: "work", Quota: &reading,
		})
		if len(recorder.readings) != 1 {
			t.Fatalf("recorded = %+v", recorder.readings)
		}
		got := recorder.readings[0]
		if got.quota != reading || got.provider != harness.ProviderCodex || got.accountID != "work" {
			t.Fatalf("recorded = %+v", got)
		}
		if got.ctx.Err() != nil {
			t.Fatalf("recording context is cancelled: %v", got.ctx.Err())
		}
		if got.ctx.Value(contextKey{}) != "run" {
			t.Fatal("recording context lost request values")
		}
		if got.ctx.Done() != nil {
			t.Fatal("a later prompt cancellation can still cancel quota persistence")
		}
	}
}

func TestRecordQuotaDropsOtherEventsAndMissingReadings(t *testing.T) {
	recorder := &recordingQuota{}
	service := &Service{quota: recorder}
	reading := &harness.Quota{Window: harness.QuotaWindowWeekly}
	service.recordQuota(context.Background(), harness.Event{Type: harness.EventRunCompleted, Quota: reading})
	service.recordQuota(context.Background(), harness.Event{Type: harness.EventQuotaUpdated})
	if len(recorder.readings) != 0 {
		t.Fatalf("unexpected readings: %+v", recorder.readings)
	}
	service.quota = nil
	service.recordQuota(context.Background(), harness.Event{Type: harness.EventQuotaUpdated, Quota: reading})
	if _, ok := chatEventFromAgentEvent(harness.Event{Type: harness.EventQuotaUpdated, Quota: reading}); ok {
		t.Fatal("quota observation must not become a persisted chat event")
	}
}
