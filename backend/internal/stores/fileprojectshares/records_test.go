package fileprojectshares

import (
	"encoding/json"
	"testing"

	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
)

func TestSharesFileJSONContract(t *testing.T) {
	encoded, err := json.Marshal(sharesFile{
		Shares: []shareRecord{{
			ID:        "1f2e3d4c",
			TokenHash: "digest",
			Port:      3000,
			Label:     "client demo",
			CreatedBy: "owner@example.com",
			CreatedAt: 10,
			ExpiresAt: 20,
			RevokedAt: 30,
		}},
		UpdatedAt: 40,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"shares":[{"id":"1f2e3d4c","tokenHash":"digest","port":3000,"label":"client demo","createdBy":"owner@example.com","createdAt":10,"expiresAt":20,"revokedAt":30}],"updatedAt":40}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}

	encoded, err = json.Marshal(sharesFile{
		Shares:    []shareRecord{{ID: "1f2e3d4c", Port: 3000}},
		UpdatedAt: 40,
	})
	if err != nil {
		t.Fatalf("Marshal omitted fields: %v", err)
	}
	want = `{"shares":[{"id":"1f2e3d4c","tokenHash":"","port":3000,"createdAt":0,"expiresAt":0}],"updatedAt":40}`
	if string(encoded) != want {
		t.Fatalf("JSON with omitted fields = %s, want %s", encoded, want)
	}
}

func TestShareRecordMappersPreserveNilAndEmptySlices(t *testing.T) {
	if got := shareRecordsFromService(nil); got != nil {
		t.Fatalf("shareRecordsFromService(nil) = %#v, want nil", got)
	}
	if got := shareRecordsToService(nil); got != nil {
		t.Fatalf("shareRecordsToService(nil) = %#v, want nil", got)
	}

	if got := shareRecordsFromService([]serviceshare.Record{}); got == nil || len(got) != 0 {
		t.Fatalf("shareRecordsFromService(empty) = %#v, want non-nil empty", got)
	}
	if got := shareRecordsToService([]shareRecord{}); got == nil || len(got) != 0 {
		t.Fatalf("shareRecordsToService(empty) = %#v, want non-nil empty", got)
	}
}
