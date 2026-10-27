package api

import (
	"errors"
	"time"
)

var (
	ErrInvalidCron     = errors.New("invalid five-field cron expression")
	ErrInvalidTimezone = errors.New("invalid IANA timezone")
)

type Task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	ChatID      string `json:"chatId"`
	OwnerEmail  string `json:"ownerEmail"`
	Kind        string `json:"kind"`
	At          string `json:"at,omitempty"`
	Cron        string `json:"cron,omitempty"`
	Timezone    string `json:"timezone"`
	Enabled     bool   `json:"enabled"`
	Archived    bool   `json:"archived"`
	NextRunAt   int64  `json:"nextRunAt,omitempty"`
	RunCount    int    `json:"runCount"`
	MaxRuns     int    `json:"maxRuns,omitempty"`
	ActiveRunID string `json:"activeRunId,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	LastRunAt   int64  `json:"lastRunAt,omitempty"`
}

func nextOccurrence(t Task, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(t.Timezone)
	if err != nil {
		return time.Time{}, ErrInvalidTimezone
	}
	switch t.Kind {
	case "once":
		at, err := time.Parse(time.RFC3339Nano, t.At)
		if err != nil || !at.After(after) {
			return time.Time{}, errors.New("at must be a future RFC3339 time with offset")
		}
		if t.Cron != "" {
			return time.Time{}, errors.New("choose at or cron")
		}
		return at, nil
	case "cron":
		if t.At != "" {
			return time.Time{}, errors.New("choose at or cron")
		}
		return nextCron(t.Cron, after, loc)
	default:
		return time.Time{}, errors.New("kind must be once or cron")
	}
}
