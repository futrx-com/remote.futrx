package filechat

import "testing"

func jsonlIndexForTest(t testing.TB, store *Store) *chatEventIndex {
	t.Helper()
	projection, ok := store.transcript.(*jsonlTranscriptProjection)
	if !ok {
		t.Fatalf("transcript projection is %T, want JSONL", store.transcript)
	}
	return projection.index
}

func sqliteDBForTest(t testing.TB, store *Store) *chatStoreDB {
	t.Helper()
	projection, ok := store.transcript.(*sqliteTranscriptProjection)
	if !ok {
		t.Fatalf("transcript projection is %T, want SQLite", store.transcript)
	}
	return projection.db
}
