package prompt

import (
	"context"
	"testing"
)

func TestPromptEventsPersistInitiatingUserForSuccessfulAndFailedRuns(t *testing.T) {
	for _, fail := range []bool{false, true} {
		provider := &usageProvider{fail: fail}
		service, store, meta := newUsagePromptService(t, provider, &recordingLedger{})
		for _, email := range []string{"alice@example.com", "bob@example.com"} {
			handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "hello", Actor: Actor{Email: email}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for range handle.Done {
			}
		}
		events, err := store.ReadEvents(context.Background(), meta.ID)
		if err != nil {
			t.Fatal(err)
		}
		var actor string
		var users, terminal int
		for _, event := range events {
			if event.Type == "user" {
				users++
				if users == 1 {
					actor = "alice@example.com"
				} else {
					actor = "bob@example.com"
				}
			}
			if event.UserEmail != actor || event.TurnID == "" {
				t.Fatalf("unattributed run event: %#v", event)
			}
			if event.Type == "complete" || event.Type == "error" {
				terminal++
			}
		}
		if users != 2 || terminal < 2 {
			t.Fatalf("missing run events: %#v", events)
		}
	}
}

func TestScheduledPromptUsesStoredOwnerAttribution(t *testing.T) {
	service, store, meta := newUsagePromptService(t, &usageProvider{}, &recordingLedger{}, WithScheduleToolIssuer(stubScheduleTools{}))
	handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "scheduled", Actor: Actor{Email: "owner@example.com"}, ScheduledTaskID: "task"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range handle.Done {
	}
	events, err := store.ReadEvents(context.Background(), meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("no events")
	}
	for _, event := range events {
		if event.UserEmail != "owner@example.com" || event.ScheduledTaskID != "task" {
			t.Fatalf("lost scheduled attribution: %#v", event)
		}
	}
}
