package sqlitechat

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestAppendItemPreservesCurrentAndPreviousNativePayloads(t *testing.T) {
	for _, test := range []struct {
		name      string
		startType string
		endType   string
		startRaw  string
		endRaw    string
	}{
		{
			name:      "web search completion grows",
			startType: "tool_use_start", endType: "tool_use_end",
			startRaw: `{"threadId":"thread-1","turnId":"turn-1","item":{"id":"search-1","type":"webSearch","query":"SQLite"}}`,
			endRaw:   `{"threadId":"thread-1","turnId":"turn-1","item":{"id":"search-1","type":"webSearch","query":"SQLite persistence and backups"}}`,
		},
		{
			name:      "equal length payload changes",
			startType: "tool_use_start", endType: "tool_use_end",
			startRaw: `{"item":{"id":"tool-1","status":"started"}}`,
			endRaw:   `{"item":{"id":"tool-1","status":"success"}}`,
		},
		{
			name:      "interaction resolution grows",
			startType: "interaction_request", endType: "interaction_resolved",
			startRaw: `{"requestId":"input-1","status":"pending"}`,
			endRaw:   `{"requestId":"input-1","status":"resolved","answers":{"environment":["qa"]}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := testStore(t)
			makeChat(t, store, "abcd")
			original := []chat.Event{
				{T: 1, Type: test.startType, TurnID: "turn-1", ID: "item-1", Provider: chat.ProviderCodex,
					Name: "WebSearch", Input: json.RawMessage(`{"query":"SQLite"}`), Raw: json.RawMessage(test.startRaw)},
				{T: 2, Type: test.endType, TurnID: "turn-1", ID: "item-1", Provider: chat.ProviderCodex,
					Output: "Found SQLite documentation", Raw: json.RawMessage(test.endRaw)},
			}
			for i := range original {
				original[i].Native = &harness.NativeEnvelope{
					SchemaVersion: harness.NativeEnvelopeSchemaVersion, ThreadID: "thread-1", TurnID: "turn-1",
					Payload: original[i].Raw,
				}
			}
			want, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var committed []chat.Event
			for _, event := range original {
				stored, err := store.AppendEvent(testContext, "abcd", event)
				if err != nil {
					t.Fatalf("append %s: %v", event.Type, err)
				}
				committed = append(committed, stored)
			}
			got, err := json.Marshal(original)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("append mutated the caller's native payloads: %s, err %v; want %s", got, err, want)
			}
			stored, err := store.ReadEvents(testContext, "abcd")
			if err != nil || !reflect.DeepEqual(stored, committed) {
				t.Fatalf("committed events changed: %+v, err %v; want %+v", stored, err, committed)
			}
			page, err := store.ReadTranscriptPage(testContext, "abcd", chat.TranscriptPageQuery{})
			if err != nil || len(page.Turns) != 1 {
				t.Fatalf("read projected turn: %+v, err %v", page, err)
			}
			for i := range committed {
				committed[i].Native = nil
				committed[i].Raw = nil
			}
			if !reflect.DeepEqual(page.Turns[0].Events, committed) {
				t.Fatalf("projection changed item events: %+v; want %+v", page.Turns[0].Events, committed)
			}
		})
	}
}
