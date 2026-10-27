package fileprojectshares

import serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"

type sharesFile struct {
	Shares    []shareRecord `json:"shares"`
	UpdatedAt int64         `json:"updatedAt"`
}

type shareRecord struct {
	ID        string `json:"id"`
	TokenHash string `json:"tokenHash"`
	Port      int    `json:"port"`
	Label     string `json:"label,omitempty"`
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	RevokedAt int64  `json:"revokedAt,omitempty"`
}

func shareRecordsFromService(records []serviceshare.Record) []shareRecord {
	if records == nil {
		return nil
	}
	stored := make([]shareRecord, len(records))
	for index, record := range records {
		stored[index] = shareRecord{
			ID:        string(record.ID),
			TokenHash: record.TokenHash,
			Port:      record.Port,
			Label:     record.Label,
			CreatedBy: record.CreatedBy,
			CreatedAt: record.CreatedAt,
			ExpiresAt: record.ExpiresAt,
			RevokedAt: record.RevokedAt,
		}
	}
	return stored
}

func shareRecordsToService(stored []shareRecord) []serviceshare.Record {
	if stored == nil {
		return nil
	}
	records := make([]serviceshare.Record, len(stored))
	for index, record := range stored {
		records[index] = serviceshare.Record{
			ID:        serviceshare.ID(record.ID),
			TokenHash: record.TokenHash,
			Port:      record.Port,
			Label:     record.Label,
			CreatedBy: record.CreatedBy,
			CreatedAt: record.CreatedAt,
			ExpiresAt: record.ExpiresAt,
			RevokedAt: record.RevokedAt,
		}
	}
	return records
}
