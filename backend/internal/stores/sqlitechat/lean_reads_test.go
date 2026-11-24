package sqlitechat

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	constants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestDuplicateToolUpdatesRemainBoundedAndPageOnce(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "turn", Text: "start"}, chat.Event{Type: "tool_use_start", TurnID: "turn", ID: "tool", Input: json.RawMessage(`{"command":"check"}`)})
	for i := 0; i < 200; i++ {
		appendEvents(t, s, "aaaa", chat.Event{Type: "tool_use_end", TurnID: "turn", ID: "tool", Output: strings.Repeat("x", 1000)})
	}
	page, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{ByteLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || len(page.Turns[0].Events) != 2 || !page.HasMore || page.NextBefore != 2 {
		t.Fatalf("tool item not bounded: %+v", page)
	}
	if page.Turns[0].Events[1].Seq != 202 {
		t.Fatal("missing latest completion")
	}
	previous, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{BeforeSeq: page.NextBefore})
	if err != nil {
		t.Fatal(err)
	}
	if len(previous.Turns) != 1 || len(previous.Turns[0].Events) != 1 || previous.Turns[0].Events[0].Type != "user" || previous.HasMore {
		t.Fatalf("repeated tool on older page: %+v", previous)
	}
}

func TestCollaborationContentComesFromRetainedEvent(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	makeChat(t, s, "bbbb")
	large := strings.Repeat("🙂 payload ", 10000)
	raw, _ := json.Marshal(map[string]any{"tools": []any{map[string]any{"name": "Read", "input": map[string]any{"text": large}, "output": large}}, "agentsStates": map[string]any{"agent\"key": map[string]any{"message": large}}})
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "one", T: 1, Text: "go"}, chat.Event{Type: "collaboration", TurnID: "one", ID: "agent", T: 2, Data: raw})
	page, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Tools []struct {
			Input struct {
				ContentRef string `json:"contentRef"`
			}
			Output    string
			OutputRef string
		}
		AgentsStates map[string]struct {
			Message    string
			MessageRef string
		}
	}
	if err = json.Unmarshal(page.Turns[0].Events[1].Data, &data); err != nil {
		t.Fatal(err)
	}
	refs := []string{data.Tools[0].Input.ContentRef, data.Tools[0].OutputRef, data.AgentsStates["agent\"key"].MessageRef}
	if len(data.Tools[0].Output) > constants.ChatTranscriptInlineFieldBytes || len(data.AgentsStates["agent\"key"].Message) > constants.ChatTranscriptInlineFieldBytes {
		t.Fatal("unbounded preview")
	}
	for _, ref := range refs {
		if ref == "" {
			t.Fatal("missing content reference")
		}
		chunk, err := s.ReadTranscriptContent(testContext, "aaaa", ref, 0, 100)
		if err != nil || len(chunk.Content) > 100 || chunk.Complete {
			t.Fatalf("invalid chunk: %+v %v", chunk, err)
		}
		if _, err = s.ReadTranscriptContent(testContext, "bbbb", ref, 0, 100); !errors.Is(err, chat.ErrTranscriptContentNotFound) {
			t.Fatalf("cross-chat reference accepted: %v", err)
		}
	}
	if err = s.RewindBranch(testContext, "aaaa", 2); err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if _, err = s.ReadTranscriptContent(testContext, "aaaa", ref, 0, 100); !errors.Is(err, chat.ErrTranscriptContentNotFound) {
			t.Fatalf("discarded content visible: %v", err)
		}
	}
}

func TestSearchFailureCannotRejectCanonicalAppendAndCanCatchUp(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	if _, err := s.db.Exec(`DROP TABLE transcript_search`); err != nil {
		t.Fatal(err)
	}
	appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", Text: "durable even when search is unavailable"})
	if _, err := s.SearchTranscript(testContext, "aaaa", "durable", 10); err == nil {
		t.Fatal("search failure was hidden")
	}
	if _, err := s.db.Exec(searchSchema); err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchTranscript(testContext, "aaaa", "durable", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("search did not resume: %+v %v", hits, err)
	}
	events, err := s.ReadEvents(testContext, "aaaa")
	if err != nil || len(events) != 1 {
		t.Fatalf("canonical append lost: %+v %v", events, err)
	}
}

func TestLargeSingleTextEventSearchAndForkPrefix(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	text := strings.Repeat("🙂 alpha ", 6000) + "midneedle " + strings.Repeat("omega ", 6000)
	appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: text}, chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "futureword"})
	hits, err := s.SearchTranscript(testContext, "aaaa", "midneedle", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("large text indexing: %+v %v", hits, err)
	}
	if _, err = s.ForkChat(testContext, "aaaa", chat.Meta{ID: "bbbb"}, 1); err != nil {
		t.Fatal(err)
	}
	hits, err = s.SearchTranscript(testContext, "bbbb", "midneedle", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("fork prefix lost: %+v %v", hits, err)
	}
	hits, err = s.SearchTranscript(testContext, "bbbb", "futureword", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("fork future leaked: %+v %v", hits, err)
	}
}

// Indexing an oversized answer must scale linearly with its size. Re-extracting
// the payload per chunk was quadratic and timed searches out behind the proxy.
func TestLargeTextEventIndexingScalesLinearly(t *testing.T) {
	indexTime := func(megabytes int) time.Duration {
		s := testStore(t)
		makeChat(t, s, "aaaa")
		text := strings.Repeat("🙂 alpha ", megabytes*1024*1024/11) + "tailneedle"
		appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: text})
		start := time.Now()
		hits, err := s.SearchTranscript(testContext, "aaaa", "tailneedle", 10)
		if err != nil || len(hits) != 1 {
			t.Fatalf("%dMB answer: %+v %v", megabytes, hits, err)
		}
		return time.Since(start)
	}
	small, large := indexTime(1), indexTime(8)
	// Linear indexing costs ~8x here; the quadratic path cost ~64x.
	if large > 24*small {
		t.Fatalf("8MB answer took %v, 1MB took %v: indexing is superlinear", large, small)
	}
}

func TestCollaborationPreservesOpaqueJSONShapes(t *testing.T) {
	for _, raw := range []string{
		`"opaque"`, `42`, `true`, `null`, `["opaque",{"future":true}]`,
		`{"tools":"opaque","agentsStates":["future"]}`,
		`{"tools":["opaque",true,false,null,42,["future"],{"input":"a string","output":{"future":true}},{"input":null,"output":false}]}`,
		`{"agentsStates":{"opaque":"text","flag":true,"empty":null,"future":["x"],"state":{"message":{"future":true}},"number":{"message":42}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			s := testStore(t)
			makeChat(t, s, "aaaa")
			appendEvents(t, s, "aaaa", chat.Event{Type: "collaboration", ID: "agent", Data: json.RawMessage(raw)})
			page, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{})
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err = json.Unmarshal([]byte(raw), &want); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(page.Turns[0].Events[0].Data, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("opaque data changed: got %#v want %#v", got, want)
			}
		})
	}
}

func TestConcurrentFirstSearchWhileTextContinues(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "anchorphrase "})
	start := make(chan struct{})
	failures := make(chan error, 8)
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		for i := 0; i < 100; i++ {
			if _, err := s.AppendEvent(testContext, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "continued text "}); err != nil {
				failures <- err
				return
			}
		}
	}()
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for j := 0; j < 10; j++ {
				hits, err := s.SearchTranscript(testContext, "aaaa", "anchorphrase", 10)
				if err != nil {
					failures <- err
					return
				}
				if len(hits) == 0 {
					failures <- fmt.Errorf("concurrent search lost saved prefix")
					return
				}
			}
		}()
	}
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}

func TestSearchHitsReportSourceKindAndTime(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa",
		chat.Event{Type: "user", TurnID: "one", Text: "please check kindneedle", T: 100},
		chat.Event{Type: "thinking", TurnID: "one", MessageID: "think", Text: "kindneedle reasoning", T: 200},
		chat.Event{Type: "tool_use_start", TurnID: "one", ID: "tool", Name: "Bash", T: 300},
		chat.Event{Type: "tool_use_end", TurnID: "one", ID: "tool", Output: "kindneedle output", T: 400},
		chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "answer", Text: "kindneedle ", T: 500},
		chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "answer", Text: "answer", T: 600},
	)
	hits, err := s.SearchTranscript(testContext, "aaaa", "kindneedle", 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, hit := range hits {
		got[hit.Kind] = hit.T
	}
	want := map[string]int64{"user": 100, "thinking": 200, "tool_use_end": 400, "assistant_text": 500}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds and times: got %v want %v (hits %+v)", got, want, hits)
	}
}

func TestSearchMatchesLastWordAsPrefix(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa",
		chat.Event{Type: "user", TurnID: "one", Text: "please run the integration tests", T: 100},
		chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "Testing finished: zebra-start passed", T: 200},
	)
	for query, want := range map[string]int{
		"t":                2, // "the", "tests", "Testing"
		"integ":            1,
		"integration te":   1, // a phrase whose last word is still being typed
		"zebra-sta":        1,
		"tests passed":     0, // earlier words still match whole
		"-":                0,
		`"`:                0,
		"run the integrat": 1,
	} {
		hits, err := s.SearchTranscript(testContext, "aaaa", query, 10)
		if err != nil {
			t.Fatalf("%q: %v", query, err)
		}
		if len(hits) != want {
			t.Fatalf("%q: got %d hits, want %d: %+v", query, len(hits), want, hits)
		}
	}
}

func TestSearchPagesReachTheOldestMatch(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	for i := 0; i < 250; i++ {
		appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: fmt.Sprintf("turn-%03d", i), Text: fmt.Sprintf("pagedword number %d", i), T: int64(1000 + i)})
	}
	var all []chat.SearchHit
	before := int64(0)
	for page := 0; page < 10; page++ {
		hits, err := s.SearchTranscriptPage(testContext, "aaaa", "pagedword", 100, before)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			break
		}
		all = append(all, hits...)
		before = hits[len(hits)-1].StartSeq
	}
	if len(all) != 250 {
		t.Fatalf("paged through %d hits, want 250", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].StartSeq >= all[i-1].StartSeq {
			t.Fatalf("hits not strictly newest first at %d: %d then %d", i, all[i-1].StartSeq, all[i].StartSeq)
		}
	}
	if all[0].T != 1249 || all[len(all)-1].T != 1000 {
		t.Fatalf("pages span %d..%d, want newest 1249 to oldest 1000", all[0].T, all[len(all)-1].T)
	}
}

func TestTranscriptWindowAroundAnchor(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	userSeq := map[int]int64{}
	for i := 1; i <= 30; i++ {
		turn := fmt.Sprintf("turn-%02d", i)
		seq, err := s.AppendEvent(testContext, "aaaa", chat.Event{Type: "user", TurnID: turn, Text: fmt.Sprintf("question %d", i), T: int64(i * 10)})
		if err != nil {
			t.Fatal(err)
		}
		userSeq[i] = seq.Seq
		appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: turn, MessageID: turn, Text: fmt.Sprintf("answer %d", i), T: int64(i*10 + 1)})
	}
	turnsOf := func(page chat.TranscriptPage) []string {
		var ids []string
		for _, turn := range page.Turns {
			ids = append(ids, turn.ID)
		}
		return ids
	}

	page, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{Limit: 7, AnchorSeq: userSeq[15], NewerTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	got := turnsOf(page)
	if len(got) != 7 || got[0] != "turn-12" || got[6] != "turn-18" {
		t.Fatalf("window around turn 15: got %v, want turn-12..turn-18", got)
	}
	if page.NewerFrom != userSeq[19] || !page.HasMore {
		t.Fatalf("window cursors: newerFrom=%d (want %d) hasMore=%v", page.NewerFrom, userSeq[19], page.HasMore)
	}

	// Near the end there are fewer newer turns than asked for: the page ends
	// with the transcript and reports nothing newer.
	page, err = s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{Limit: 7, AnchorSeq: userSeq[29], NewerTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	got = turnsOf(page)
	if page.NewerFrom != 0 || got[len(got)-1] != "turn-30" {
		t.Fatalf("window at the end: got %v newerFrom=%d", got, page.NewerFrom)
	}
}

func TestSearchUntilBoundsMatchesByTime(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	for i := 1; i <= 5; i++ {
		appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: fmt.Sprintf("turn-%d", i), Text: "timedword", T: int64(i * 100)})
	}
	hits, err := s.SearchTranscriptUntil(testContext, "aaaa", "timedword", 10, 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].T != 300 || hits[2].T != 100 {
		t.Fatalf("hits until t=300: %+v", hits)
	}
}
