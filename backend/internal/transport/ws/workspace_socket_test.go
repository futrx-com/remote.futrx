package wstransport

import (
	"context"
	"net/http"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"
)

// stubVisibility is a test double for WorkspaceVisibility.
type stubVisibility struct {
	// access maps "projectID:email" → allowed
	access map[string]bool
}

func (s *stubVisibility) CallerAndAdmin(_ context.Context, _ *http.Request) (string, bool, error) {
	return "", false, nil // unused by filterEvent
}

func (s *stubVisibility) HasAccess(_ context.Context, pid serviceproject.ID, email string) (bool, error) {
	key := string(pid) + ":" + email
	ok, found := s.access[key]
	if !found {
		return false, nil
	}
	return ok, nil
}

func makeProject(id string) *serviceproject.Meta {
	return &serviceproject.Meta{ID: serviceproject.ID(id), Name: "Project " + id}
}

func makeChat(id, projectID string) *servicechat.Meta {
	return &servicechat.Meta{ID: servicechat.ID(id), ProjectID: servicechat.ProjectID(projectID)}
}

func TestFilterEvent_AdminBypassesFiltering(t *testing.T) {
	// filterEvent is only called for non-admin users. For admin users the
	// caller (handle) skips filtering entirely. This test verifies that
	// filterEvent correctly passes through events when it IS called with
	// a visibility that grants access.
	vis := &stubVisibility{access: map[string]bool{
		"proj1:admin@test.com": true,
	}}
	allowed := map[serviceproject.ID]struct{}{
		"proj1": {},
	}

	ev := workspacehub.Event{
		Type:    "project.upsert",
		Project: makeProject("proj1"),
	}
	out, ok := filterEvent(vis, "admin@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected event to be forwarded for user with access")
	}
	if out.Type != "project.upsert" {
		t.Fatalf("expected project.upsert, got %s", out.Type)
	}
}

func TestFilterEvent_NonAdminReceivesAccessibleProjectUpsert(t *testing.T) {
	vis := &stubVisibility{access: map[string]bool{
		"proj1:user@test.com": true,
	}}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type:    "project.upsert",
		Project: makeProject("proj1"),
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected event to be forwarded for authorized user")
	}
	if out.Type != "project.upsert" {
		t.Fatalf("expected project.upsert, got %s", out.Type)
	}
	// proj1 should now be in the allowed set
	if _, exists := allowed["proj1"]; !exists {
		t.Fatal("expected proj1 to be added to allowed set")
	}
}

func TestFilterEvent_NonAdminBlockedFromUnauthorizedProject(t *testing.T) {
	vis := &stubVisibility{access: map[string]bool{
		"proj_secret:user@test.com": false,
	}}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type:    "project.upsert",
		Project: makeProject("proj_secret"),
	}
	_, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if ok {
		t.Fatal("expected event to be suppressed for unauthorized user")
	}
}

func TestFilterEvent_RevokedAccessSendsDelete(t *testing.T) {
	vis := &stubVisibility{access: map[string]bool{
		"proj1:user@test.com": false, // access revoked
	}}
	// User previously had access
	allowed := map[serviceproject.ID]struct{}{
		"proj1": {},
	}

	ev := workspacehub.Event{
		Type:    "project.upsert",
		Project: makeProject("proj1"),
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected a synthetic delete event to be generated")
	}
	if out.Type != "project.delete" {
		t.Fatalf("expected project.delete (synthetic), got %s", out.Type)
	}
	if out.ID != "proj1" {
		t.Fatalf("expected delete for proj1, got %s", out.ID)
	}
	// proj1 should be removed from allowed
	if _, exists := allowed["proj1"]; exists {
		t.Fatal("expected proj1 to be removed from allowed set after revocation")
	}
}

func TestFilterEvent_ChatUpsertForAccessibleProject(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{
		"proj1": {},
	}

	ev := workspacehub.Event{
		Type: "chat.upsert",
		Chat: makeChat("chat1", "proj1"),
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected chat event to be forwarded for accessible project")
	}
	if out.Type != "chat.upsert" {
		t.Fatalf("expected chat.upsert, got %s", out.Type)
	}
}

func TestFilterEvent_ChatUpsertBlockedForInaccessibleProject(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{
		"proj1": {},
	}

	ev := workspacehub.Event{
		Type: "chat.upsert",
		Chat: makeChat("chat_secret", "proj_secret"),
	}
	_, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if ok {
		t.Fatal("expected chat event to be suppressed for inaccessible project")
	}
}

func TestFilterEvent_LooseChatAlwaysForwarded(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type: "chat.upsert",
		Chat: makeChat("loose_chat", ""),
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected loose chat (no project) to be forwarded")
	}
	if out.Type != "chat.upsert" {
		t.Fatalf("expected chat.upsert, got %s", out.Type)
	}
}

func TestFilterEvent_ProjectDeleteAlwaysForwarded(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{
		"proj1": {},
	}

	ev := workspacehub.Event{
		Type: "project.delete",
		ID:   "proj1",
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected project.delete to be forwarded")
	}
	if out.Type != "project.delete" {
		t.Fatalf("expected project.delete, got %s", out.Type)
	}
}

func TestFilterEvent_ChatDeleteAlwaysForwarded(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type: "chat.delete",
		ID:   "chat42",
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected chat.delete to always be forwarded")
	}
	if out.Type != "chat.delete" {
		t.Fatalf("expected chat.delete, got %s", out.Type)
	}
}

func TestFilterEvent_NilProjectUpsertSuppressed(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type:    "project.upsert",
		Project: nil,
	}
	_, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if ok {
		t.Fatal("expected nil-project upsert to be suppressed")
	}
}

func TestFilterEvent_NilChatUpsertSuppressed(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type: "chat.upsert",
		Chat: nil,
	}
	_, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if ok {
		t.Fatal("expected nil-chat upsert to be suppressed")
	}
}

func TestFilterEvent_UnknownEventTypeForwarded(t *testing.T) {
	vis := &stubVisibility{}
	allowed := map[serviceproject.ID]struct{}{}

	ev := workspacehub.Event{
		Type: "system.ping",
	}
	out, ok := filterEvent(vis, "user@test.com", allowed, ev)
	if !ok {
		t.Fatal("expected unknown event types to be forwarded")
	}
	if out.Type != "system.ping" {
		t.Fatalf("expected system.ping, got %s", out.Type)
	}
}
