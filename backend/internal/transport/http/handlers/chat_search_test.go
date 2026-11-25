package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

func TestWorkspaceSearchRoutesScopeMetadataCountsAndMessages(t *testing.T) {
	f := newMembershipRoutesFixture(t)
	store, err := sqlitechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, meta := range []servicechat.Meta{
		{ID: "aaaa", Title: "Private chat", ProjectID: servicechat.ProjectID(f.projectID), Provider: servicechat.ProviderClaude},
		{ID: "bbbb", Title: "Public chat", Provider: servicechat.ProviderCodex},
	} {
		if _, err = store.Create(ctx, meta); err != nil {
			t.Fatal(err)
		}
		if _, err = store.AppendEvent(ctx, meta.ID, servicechat.Event{Type: "user", Text: "searchneedle", T: 1000}); err != nil {
			t.Fatal(err)
		}
	}

	chats := servicechat.New(store, nil, nil, nil, servicechat.WithTranscriptSearchSource(store))
	mux := http.NewServeMux()
	NewChatHandler(chats, servicechat.NewAccessService(chats, f.projects), f.auth, nil, nil).
		WithSearchProjects(f.projects).RegisterRoutes(mux)
	f.handler = httpmiddleware.NewAuth(f.auth).Wrap(mux)

	response := f.do(t, "outsider@example.com", "POST", "/api/chats/search-metadata", `{"withCounts":true}`)
	var metadata servicechat.WorkspaceSearchResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &metadata) != nil ||
		metadata.Total != 1 || len(metadata.Hits) != 1 || metadata.Hits[0].Chat.ID != "bbbb" ||
		metadata.Counts["provider"]["claude"] != 0 {
		t.Fatalf("outsider metadata: %d %s", response.Code, response.Body)
	}

	response = f.do(t, "member@example.com", "POST", "/api/chats/search-metadata", `{"query":"members only"}`)
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &metadata) != nil ||
		len(metadata.Hits) != 1 || metadata.Hits[0].Chat.ID != "aaaa" || metadata.Hits[0].MatchedField != "project" {
		t.Fatalf("member metadata: %d %s", response.Code, response.Body)
	}

	messageSearch := func(email string, want servicechat.ID) {
		t.Helper()
		response := f.do(t, email, "POST", "/api/chats/search", `{"query":"searchneedle","facets":{"provider":["claude"]}}`)
		var hits []servicechat.ChatSearchHit
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &hits) != nil {
			t.Fatalf("message search: %d %s", response.Code, response.Body)
		}
		if want == "" && len(hits) != 0 || want != "" && (len(hits) != 1 || hits[0].ChatID != want) {
			t.Fatalf("message scope for %s: %+v", email, hits)
		}
	}
	messageSearch("member@example.com", "aaaa")
	messageSearch("outsider@example.com", "")

	response = f.do(t, "member@example.com", "GET", "/api/chats/search?q=searchneedle", "")
	var hits []servicechat.ChatSearchHit
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &hits) != nil || len(hits) != 2 {
		t.Fatalf("legacy message search: %d %s", response.Code, response.Body)
	}
	response = f.do(t, "member@example.com", "POST", "/api/chats/search-metadata", `{"facets":{"invalid":[]}}`)
	if response.Code != 400 {
		t.Fatalf("invalid filter: %d %s", response.Code, response.Body)
	}
}
