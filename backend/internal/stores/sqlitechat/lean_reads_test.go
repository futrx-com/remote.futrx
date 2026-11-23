package sqlitechat

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

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
