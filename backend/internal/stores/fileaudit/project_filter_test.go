package fileaudit

import (
	"bytes"
	"context"
	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	"strings"
	"testing"
	"time"
)

func TestProjectActorActionAndDateFiltersSurviveReopenAndExport(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	svc := serviceaudit.New(store, serviceaudit.WithClock(func() time.Time { return now }))
	ctx := serviceaudit.WithCaller(context.Background(), serviceaudit.Caller{Actor: serviceaudit.Actor{Email: "member@example.com"}})
	for _, project := range []string{"beef", "cafe"} {
		svc.Record(ctx, serviceaudit.Success("agent.run.start", serviceaudit.Target{Type: "chat", ID: "abcd"}, serviceaudit.Meta{"projectId": project, "accountId": "work"}))
		svc.Record(ctx, serviceaudit.Success("project.secret.set", serviceaudit.Target{Type: "project", ID: project}, serviceaudit.Meta{"key": "TOKEN"}))
	}
	store, err = New(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc = serviceaudit.New(store)
	query := serviceaudit.Query{Project: "beef", Actor: "member@example.com", Action: "agent.", From: now.Add(-time.Second), To: now.Add(time.Second)}
	page, err := svc.Query(ctx, query)
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	if page.Entries[0].ProjectID != "beef" || page.Entries[0].Meta["accountId"] != "work" {
		t.Fatal(page.Entries[0])
	}
	var out bytes.Buffer
	if err := svc.ExportQuery(ctx, query, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\n") != 1 || strings.Contains(out.String(), "cafe") || strings.Contains(out.String(), "secret.set") {
		t.Fatal(out.String())
	}
}
