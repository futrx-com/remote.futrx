package chat

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

// fakeSearch serves fixed hits per chat, newest first, honouring maxT.
type fakeSearch map[ID][]SearchHit

func (f fakeSearch) SearchTranscriptPage(context.Context, ID, string, int, int64) ([]SearchHit, error) {
	return nil, errors.New("unused")
}

func (f fakeSearch) SearchTranscriptUntil(_ context.Context, id ID, _ string, limit int, maxT int64) ([]SearchHit, error) {
	hits, ok := f[id]
	if !ok {
		return nil, ErrTranscriptUnavailable
	}
	out := []SearchHit{}
	for _, hit := range hits {
		if maxT == 0 || hit.T <= maxT {
			out = append(out, hit)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func TestSearchChatsPagesAcrossChatsByTime(t *testing.T) {
	source := fakeSearch{
		"aaaa": {{StartSeq: 9, T: 900}, {StartSeq: 5, T: 500}, {StartSeq: 1, T: 100}},
		// Same millisecond as a hit in aaaa: the cursor must not skip either.
		"bbbb": {{StartSeq: 7, T: 700}, {StartSeq: 4, T: 500}, {StartSeq: 2, T: 200}},
	}
	service := New(nil, nil, nil, nil, WithTranscriptSearchSource(source))
	ids := []ID{"aaaa", "bbbb", "cccc"} // cccc has no transcript and is skipped

	var got []string
	cursor := ChatSearchCursor{}
	for page := 0; page < 10; page++ {
		hits, err := service.SearchChats(context.Background(), ids, "word", 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			break
		}
		for _, hit := range hits {
			got = append(got, string(hit.ChatID)+":"+strconv.FormatInt(hit.T, 10))
		}
		last := hits[len(hits)-1]
		cursor = ChatSearchCursor{T: last.T, ChatID: last.ChatID, StartSeq: last.StartSeq}
	}
	want := []string{"aaaa:900", "bbbb:700", "bbbb:500", "aaaa:500", "bbbb:200", "aaaa:100"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestParseChatSearchCursor(t *testing.T) {
	cursor, err := ParseChatSearchCursor("1500:abcd:42")
	if err != nil || cursor != (ChatSearchCursor{T: 1500, ChatID: "abcd", StartSeq: 42}) {
		t.Fatalf("cursor %+v %v", cursor, err)
	}
	for _, raw := range []string{"1500", "x:abcd:1", "1500:not-hex!:1", "0:abcd:1"} {
		if _, err := ParseChatSearchCursor(raw); !errors.Is(err, ErrInvalidSearchCursor) {
			t.Fatalf("%q: want ErrInvalidSearchCursor, got %v", raw, err)
		}
	}
}
