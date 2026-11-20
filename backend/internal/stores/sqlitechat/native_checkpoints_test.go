package sqlitechat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func completeNativeTestRun(t *testing.T, s *Store, id chat.ID, provider chat.Provider, account, nativeID, cursor string, timestamp int64) int64 {
	t.Helper()
	run := prepareNativeTestRun(t, s, id, provider, account, fmt.Sprintf("run-%d", timestamp))
	appendEvents(t, s, string(id),
		chat.Event{T: timestamp, TurnID: run.RunID, Type: "user", Text: run.RunID + " request", Provider: provider, AccountID: account},
		chat.Event{T: timestamp + 1, TurnID: run.RunID, Type: "assistant_text", Text: run.RunID + " response", Provider: provider, AccountID: account},
		chat.Event{T: timestamp + 2, TurnID: run.RunID, Type: "complete", Provider: provider, AccountID: account},
	)
	acceptNativeTestRun(t, s, run, nativeID)
	if err := s.FinishRun(testContext, run.RunReference, chat.RunOutcome{Status: "completed", NativeSessionID: nativeID, NativeCheckpoint: cursor}); err != nil {
		t.Fatal(err)
	}
	_, tail, err := activeBranch(testContext, s.db, id)
	if err != nil {
		t.Fatal(err)
	}
	return tail
}

func prepareNativeTestRun(t *testing.T, s *Store, id chat.ID, provider chat.Provider, account, runID string) chat.PreparedRun {
	t.Helper()
	run, err := s.PrepareRun(testContext, chat.RunPreparation{
		ChatID: id, RunID: runID, Provider: provider, AccountID: account, AllowNativeFork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func acceptNativeTestRun(t *testing.T, s *Store, run chat.PreparedRun, nativeID string) {
	t.Helper()
	if err := s.RecordRunInput(testContext, run.RunReference, "prompt_context_v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AcceptRunInput(testContext, run.RunReference, chat.InputAcceptance{NativeSessionID: nativeID, Evidence: "native_ack"}); err != nil {
		t.Fatal(err)
	}
}

func forkNativeTestChat(t *testing.T, s *Store, source, target chat.ID, cutoff int64) {
	t.Helper()
	if _, err := s.ForkChat(testContext, source, chat.Meta{ID: target, Provider: chat.ProviderClaude}, cutoff); err != nil {
		t.Fatal(err)
	}
}

func nativeTestCheckpointCount(t *testing.T, s *Store) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM events WHERE event_type='system' AND json_extract(payload_json,'$.subtype')='native_checkpoint'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestNativeCheckpointForkSurvivesSourceGrowthAndReopen(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	makeChat(t, s, "aaaa")
	cutoff := completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "account", "native-source", "claude:first", 10)
	forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
	completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "account", "native-source", "claude:later", 20)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	run := prepareNativeTestRun(t, s, "bbbb", chat.ProviderClaude, "account", "fork-run")
	if run.ResumeID != "native-source" || run.NativeCheckpoint != "claude:first" || run.NativeConversationID != "aaaa" {
		t.Fatalf("fork lost its original native checkpoint: %+v", run)
	}
	if run.History.AfterSeq != cutoff || run.History.ThroughSeq != cutoff || len(run.History.Records) != 0 {
		t.Fatalf("source growth changed fork history: %+v", run.History)
	}
	page, err := s.ReadTranscriptPage(testContext, "bbbb", chat.TranscriptPageQuery{})
	if err != nil || len(page.Turns) != 1 {
		t.Fatalf("hidden checkpoint created a visible turn: %+v, %v", page, err)
	}
	for _, event := range page.Turns[0].Events {
		if event.Subtype == nativeCheckpointSubtype {
			t.Fatal("native checkpoint leaked into transcript")
		}
	}
}

func TestNativeCheckpointIndexAddedWhenExistingV2StoreReopens(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s != nil {
			s.Close()
		}
	})
	makeChat(t, s, "aaaa")
	completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "native", "claude:first", 10)
	if _, err = s.db.Exec("DROP INDEX events_native_checkpoints"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='events_native_checkpoints'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("existing v2 store did not gain native checkpoint index: %d, %v", count, err)
	}
	forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
	run := prepareNativeTestRun(t, s, "bbbb", chat.ProviderClaude, "", "fork")
	if run.ResumeID != "native" || run.NativeCheckpoint != "claude:first" {
		t.Fatalf("index recreation lost checkpoint evidence: %+v", run)
	}
}

func TestNativeCheckpointRewindRetainsOnlyEarlierNativeBoundary(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial=%t", partial), func(t *testing.T) {
			s := testStore(t)
			makeChat(t, s, "aaaa")
			first := completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "source", "claude:first", 10)
			completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "source", "claude:discarded", 20)
			before := int64(20)
			if partial {
				before = 22
			}
			if err := s.RewindBranch(testContext, "aaaa", before); err != nil {
				t.Fatal(err)
			}
			run := prepareNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "rewound")
			if run.ResumeID != "source" || run.NativeCheckpoint != "claude:first" || run.History.AfterSeq != first {
				t.Fatalf("rewind used discarded native state: %+v", run)
			}
			wantRecords := 0
			if partial {
				wantRecords = 2
			}
			if len(run.History.Records) != wantRecords {
				t.Fatalf("retained suffix: %+v", run.History)
			}
			for _, record := range run.History.Records {
				if record.TurnID != "run-20" {
					t.Fatalf("replayed history already present in native checkpoint: %+v", record)
				}
			}
		})
	}
}

func TestNativeCheckpointNestedBranchesKeepHarnessAndAccountBoundaries(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	first := completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "one", "claude-one", "claude:first", 10)
	second := completeNativeTestRun(t, s, "aaaa", chat.ProviderCodex, "one", "codex-one", "codex:first", 20)
	third := completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "two", "claude-two", "claude:other-account", 30)
	forkNativeTestChat(t, s, "aaaa", "bbbb", third)
	completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "one", "claude-one", "claude:later", 40)
	forkNativeTestChat(t, s, "bbbb", "cccc", second)
	for _, test := range []struct {
		provider chat.Provider
		account  string
		session  string
		cursor   string
		after    int64
		records  int
	}{
		{chat.ProviderClaude, "one", "claude-one", "claude:first", first, 2},
		{chat.ProviderCodex, "one", "codex-one", "codex:first", second, 0},
		{chat.ProviderClaude, "two", "", "", 0, 4},
	} {
		run := prepareNativeTestRun(t, s, "cccc", test.provider, test.account, string(test.provider)+test.account)
		if run.ResumeID != test.session || run.NativeCheckpoint != test.cursor || run.History.AfterSeq != test.after || len(run.History.Records) != test.records {
			t.Fatalf("%s/%s inherited wrong native history: %+v", test.provider, test.account, run)
		}
		if run.History.ThroughSeq != second {
			t.Fatalf("nested branch escaped ancestor cutoff: %+v", run.History)
		}
	}
}

func TestNativeCheckpointRequiresAcceptedSuccessfulRun(t *testing.T) {
	for _, test := range []struct {
		name, status, cursor string
		accepted             bool
		wantCheckpoint       bool
	}{
		{"completed", "completed", "claude:done", true, true},
		{"failed", "failed", "claude:done", true, false},
		{"interrupted", "interrupted", "claude:done", true, false},
		{"uncertain", "uncertain", "claude:done", true, false},
		{"unaccepted", "completed", "claude:done", false, false},
		{"missing-cursor", "completed", "", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testStore(t)
			makeChat(t, s, "aaaa")
			run := prepareNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "original")
			appendEvents(t, s, "aaaa", chat.Event{T: 10, Type: "user", Text: "kept"})
			if test.accepted {
				acceptNativeTestRun(t, s, run, "native")
			}
			if err := s.FinishRun(testContext, run.RunReference, chat.RunOutcome{Status: test.status, NativeSessionID: "native", NativeCheckpoint: test.cursor}); err != nil {
				t.Fatal(err)
			}
			forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
			fork := prepareNativeTestRun(t, s, "bbbb", chat.ProviderClaude, "", "fork")
			if test.wantCheckpoint {
				if fork.ResumeID != "native" || fork.NativeCheckpoint != test.cursor || len(fork.History.Records) != 0 {
					t.Fatalf("successful checkpoint was not inherited: %+v", fork)
				}
			} else if fork.ResumeID != "" || fork.NativeCheckpoint != "" || len(fork.History.Records) != 1 {
				t.Fatalf("unsafe run was inherited: %+v", fork)
			}
		})
	}
}

func TestNativeCheckpointForkRequiresNewSessionIdentity(t *testing.T) {
	for _, nativeID := range []string{"", "source", "new-session"} {
		t.Run("accept="+nativeID, func(t *testing.T) {
			s := testStore(t)
			makeChat(t, s, "aaaa")
			completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "source", "claude:first", 10)
			forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
			run := prepareNativeTestRun(t, s, "bbbb", chat.ProviderClaude, "", "fork")
			appendEvents(t, s, "bbbb", chat.Event{T: 20, Type: "user", Text: "continue fork"})
			if err := s.RecordRunInput(testContext, run.RunReference, "prompt_context_v1"); err != nil {
				t.Fatal(err)
			}
			err := s.AcceptRunInput(testContext, run.RunReference, chat.InputAcceptance{NativeSessionID: nativeID, Evidence: "ack"})
			if (err == nil) != (nativeID == "new-session") {
				t.Fatalf("fork acceptance with session %q: %v", nativeID, err)
			}
			if err := s.FinishRun(testContext, run.RunReference, chat.RunOutcome{Status: "completed", NativeSessionID: nativeID, NativeCheckpoint: "claude:fork"}); err != nil {
				t.Fatal(err)
			}
			session := currentTestSession(t, s, "bbbb", chat.ProviderClaude, "")
			if nativeID == "new-session" {
				if session.State != "ready" || nativeTestCheckpointCount(t, s) != 2 {
					t.Fatalf("new native fork was not committed: %+v", session)
				}
			} else if session.State != "uncertain" || nativeTestCheckpointCount(t, s) != 1 {
				t.Fatalf("source native session was reused by fork: %+v", session)
			}
		})
	}
}

func TestNativeCheckpointStaleFinishCannotAppendCheckpoint(t *testing.T) {
	for _, replacement := range []string{"run", "branch"} {
		t.Run(replacement, func(t *testing.T) {
			s := testStore(t)
			makeChat(t, s, "aaaa")
			old := prepareNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "old")
			appendEvents(t, s, "aaaa", chat.Event{T: 10, Type: "user", Text: "old"})
			acceptNativeTestRun(t, s, old, "old-native")
			if replacement == "branch" {
				if err := s.RewindBranch(testContext, "aaaa", 10); err != nil {
					t.Fatal(err)
				}
			}
			next := prepareNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "new")
			if err := s.FinishRun(testContext, old.RunReference, chat.RunOutcome{Status: "completed", NativeSessionID: "old-native", NativeCheckpoint: "claude:stale"}); err != nil {
				t.Fatal(err)
			}
			if nativeTestCheckpointCount(t, s) != 0 {
				t.Fatal("stale completion appended a native checkpoint")
			}
			if session := currentTestSession(t, s, "aaaa", chat.ProviderClaude, ""); session.RunID != next.RunID || session.State != "pending" {
				t.Fatalf("stale completion modified replacement: %+v", session)
			}
		})
	}
}

func TestNativeCheckpointUncertainRunDoesNotReuseInheritedSource(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	completeNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "native", "claude:first", 10)
	forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
	run := prepareNativeTestRun(t, s, "bbbb", chat.ProviderClaude, "", "fork")
	appendEvents(t, s, "bbbb", chat.Event{T: 20, Type: "user", Text: "uncertain delivery"})
	acceptNativeTestRun(t, s, run, "new-native")
	if err := s.FinishRun(testContext, run.RunReference, chat.RunOutcome{Status: "interrupted", NativeCheckpoint: "claude:unsafe"}); err != nil {
		t.Fatal(err)
	}
	next := prepareNativeTestRun(t, s, "bbbb", chat.ProviderClaude, "", "retry")
	if next.ResumeID != "" || next.NativeCheckpoint != "" || next.History.AfterSeq != 0 || len(next.History.Records) != 3 {
		t.Fatalf("uncertain branch reused source checkpoint instead of fresh recovery: %+v", next)
	}
}

func TestNativeCheckpointFallbackForUnavailableEvidence(t *testing.T) {
	for _, test := range []struct {
		name       string
		version    int
		cursor     string
		allow      bool
		forceFresh bool
	}{
		{"unknown-version", 2, "claude:cursor", true, false},
		{"missing-cursor", 1, "", true, false},
		{"native-fork-disabled", 1, "claude:cursor", false, false},
		{"force-fresh", 1, "claude:cursor", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testStore(t)
			makeChat(t, s, "aaaa")
			appendEvents(t, s, "aaaa", chat.Event{T: 10, Type: "user", Text: "retained"})
			data, err := json.Marshal(nativeCheckpoint{Version: test.version, SessionID: "native", ConversationID: "aaaa", Cursor: test.cursor, ThroughSeq: 1})
			if err != nil {
				t.Fatal(err)
			}
			appendEvents(t, s, "aaaa", chat.Event{T: 11, Type: "system", Subtype: nativeCheckpointSubtype, Provider: chat.ProviderClaude, Data: data})
			forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
			run, err := s.PrepareRun(testContext, chat.RunPreparation{ChatID: "bbbb", RunID: "fork", Provider: chat.ProviderClaude, AllowNativeFork: test.allow, ForceFresh: test.forceFresh})
			if err != nil {
				t.Fatal(err)
			}
			if run.ResumeID != "" || run.NativeCheckpoint != "" || run.History.AfterSeq != 0 || len(run.History.Records) != 1 {
				t.Fatalf("unavailable checkpoint prevented full-history fallback: %+v", run)
			}
		})
	}
}

func TestNativeCheckpointFailureRollsBackSessionAndEventTogether(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	run := prepareNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "run")
	appendEvents(t, s, "aaaa", chat.Event{T: 10, Type: "user", Text: "retained"})
	acceptNativeTestRun(t, s, run, "native")
	if _, err := s.db.Exec(`CREATE TRIGGER reject_session_finish BEFORE UPDATE OF sessions_json ON chats BEGIN SELECT RAISE(ABORT,'reject session finish'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRun(testContext, run.RunReference, chat.RunOutcome{Status: "completed", NativeCheckpoint: "claude:done"}); err == nil {
		t.Fatal("expected checkpoint transaction failure")
	}
	if nativeTestCheckpointCount(t, s) != 0 {
		t.Fatal("checkpoint survived failed session transaction")
	}
	session := currentTestSession(t, s, "aaaa", chat.ProviderClaude, "")
	if session.State != "accepted" || session.RunID != run.RunID || session.HistoryThroughSeq != 0 {
		t.Fatalf("failed checkpoint changed resumable state: %+v", session)
	}
	_, tail, err := activeBranch(testContext, s.db, "aaaa")
	if err != nil || tail != 1 {
		t.Fatalf("failed checkpoint advanced event sequence: %d, %v", tail, err)
	}
}

func TestNativeCheckpointExcludedFromEventReadersButRetainedForMaintenance(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	run := prepareNativeTestRun(t, s, "aaaa", chat.ProviderClaude, "", "run")
	appendEvents(t, s, "aaaa",
		chat.Event{T: 10, TurnID: "run", Type: "user", Text: "request"},
		chat.Event{T: 11, TurnID: "run", Type: "system"},
		chat.Event{T: 12, TurnID: "run", Type: "system", Subtype: "other_system_event"},
		chat.Event{T: 13, TurnID: "run", Type: "provider_event", Subtype: nativeCheckpointSubtype},
		chat.Event{T: 14, TurnID: "run", Type: "assistant_text", Text: "response"},
		chat.Event{T: 15, TurnID: "run", Type: "complete"},
	)
	acceptNativeTestRun(t, s, run, "native")
	if err := s.FinishRun(testContext, run.RunReference, chat.RunOutcome{Status: "completed", NativeCheckpoint: "claude:done"}); err != nil {
		t.Fatal(err)
	}
	forkNativeTestChat(t, s, "aaaa", "bbbb", -1)
	for _, id := range []chat.ID{"aaaa", "bbbb"} {
		t.Run(string(id), func(t *testing.T) {
			events, err := s.ReadEvents(testContext, id)
			if err != nil || len(events) != 6 {
				t.Fatalf("ordinary event history changed or exposed checkpoint: %+v, %v", events, err)
			}
			if events[1].Type != "system" || events[1].Subtype != "" || events[2].Subtype != "other_system_event" || events[3].Type != "provider_event" {
				t.Fatalf("checkpoint filter removed unrelated events: %+v", events)
			}
			after, err := s.ReadEventsAfter(testContext, id, 6)
			if err != nil || len(after) != 0 {
				t.Fatalf("completed chat reconnect replays internal checkpoint: %+v, %v", after, err)
			}
			page, err := s.ReadEventsPage(testContext, id, chat.EventPageQuery{Limit: 2})
			if err != nil || page.LastSeq != 7 || !page.HasMore || page.NextBefore != 5 || !reflect.DeepEqual(page.Events, events[4:]) {
				t.Fatalf("checkpoint consumed event page budget or changed cursor: %+v, %v", page, err)
			}
			older, err := s.ReadEventsPage(testContext, id, chat.EventPageQuery{Limit: 4, BeforeSeq: page.NextBefore})
			if err != nil || older.LastSeq != 7 || older.HasMore || !reflect.DeepEqual(older.Events, events[:4]) {
				t.Fatalf("older page lost ordinary system events: %+v, %v", older, err)
			}
			window, err := s.ReadTranscriptEventWindow(testContext, id, 0, 1)
			if err != nil || window.LastSeq != 7 || !reflect.DeepEqual(window.Events, events) {
				t.Fatalf("transcript event window exposed checkpoint: %+v, %v", window, err)
			}
			var scanned []chat.Event
			if err := s.ScanEvents(testContext, id, func(event chat.Event) { scanned = append(scanned, event) }); err != nil {
				t.Fatal(err)
			}
			if len(scanned) != 7 || scanned[6].Type != "system" || scanned[6].Subtype != nativeCheckpointSubtype {
				t.Fatalf("maintenance scan lost authoritative checkpoint: %+v", scanned)
			}
		})
	}
}
