package httphandlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
)

const globalSkillManifest = "---\nname: guard\ndescription: Guard rails.\n---\n"

// globalSkillRepositoryStub is a minimal in-memory global skills library.
type globalSkillRepositoryStub struct {
	records map[string]serviceskills.GlobalRecord
}

func newGlobalSkillRepositoryStub() *globalSkillRepositoryStub {
	return &globalSkillRepositoryStub{records: map[string]serviceskills.GlobalRecord{}}
}

func (r *globalSkillRepositoryStub) List(context.Context) ([]serviceskills.GlobalRecord, error) {
	names := make([]string, 0, len(r.records))
	for name := range r.records {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]serviceskills.GlobalRecord, 0, len(names))
	for _, name := range names {
		out = append(out, r.records[name])
	}
	return out, nil
}

func (r *globalSkillRepositoryStub) Get(_ context.Context, name string) (serviceskills.GlobalRecord, error) {
	record, ok := r.records[name]
	if !ok {
		return serviceskills.GlobalRecord{}, serviceskills.ErrGlobalSkillNotFound
	}
	return record, nil
}

func (r *globalSkillRepositoryStub) Save(
	_ context.Context,
	record serviceskills.GlobalRecord,
) (serviceskills.GlobalRecord, error) {
	names := make([]string, 0, len(record.Files))
	for name := range record.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	record.FileNames = names
	r.records[record.Name] = record
	return record, nil
}

func (r *globalSkillRepositoryStub) SetAlwaysOn(
	_ context.Context,
	name string,
	alwaysOn bool,
) (serviceskills.GlobalRecord, error) {
	record, ok := r.records[name]
	if !ok {
		return serviceskills.GlobalRecord{}, serviceskills.ErrGlobalSkillNotFound
	}
	record.AlwaysOn = alwaysOn
	r.records[name] = record
	return record, nil
}

func (r *globalSkillRepositoryStub) Delete(_ context.Context, name string) error {
	if _, ok := r.records[name]; !ok {
		return serviceskills.ErrGlobalSkillNotFound
	}
	delete(r.records, name)
	return nil
}

// globalSkillAuthStore is the smallest serviceauth.Store that lets a real
// auth service issue and verify sessions in a test.
type globalSkillAuthStore struct{}

func (globalSkillAuthStore) OAuthConfig(context.Context) (serviceauth.OAuthConfig, error) {
	return serviceauth.OAuthConfig{}, serviceauth.ErrOAuthConfigNotFound
}

func (globalSkillAuthStore) SaveOAuthConfig(context.Context, serviceauth.OAuthConfig) error {
	return nil
}

func (globalSkillAuthStore) LocalAdmin(context.Context) (*serviceauth.LocalAdminCredential, error) {
	return nil, nil
}

func (globalSkillAuthStore) CreateLocalAdmin(context.Context, serviceauth.LocalAdminCredential) error {
	return nil
}

func (globalSkillAuthStore) DeleteLocalAdmin(context.Context, serviceauth.LocalAdminCredential) error {
	return nil
}

func (globalSkillAuthStore) SessionKey(context.Context) ([]byte, error) {
	return bytes.Repeat([]byte{7}, 32), nil
}

type globalSkillDirectory struct {
	admins map[string]bool
}

func (d globalSkillDirectory) IsAdmin(_ context.Context, email string) (bool, error) {
	return d.admins[strings.ToLower(email)], nil
}

func (d globalSkillDirectory) IsRegistered(_ context.Context, email string) (bool, error) {
	_, ok := d.admins[strings.ToLower(email)]
	return ok, nil
}

func (globalSkillDirectory) AddBootstrapAdmin(context.Context, string) error { return nil }

func (globalSkillDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return nil, nil
}

func newGlobalSkillTestAuth(t *testing.T, admins map[string]bool) *serviceauth.Service {
	t.Helper()
	service, err := serviceauth.New(
		context.Background(),
		globalSkillAuthStore{},
		globalSkillDirectory{admins: admins},
		func(string, string, string) serviceauth.OAuthProvider { return nil },
		"https://remote.example.test",
		bytes.Repeat([]byte{7}, 32),
	)
	if err != nil {
		t.Fatalf("build auth service: %v", err)
	}
	return service
}

func newGlobalSkillHandler(t *testing.T) (*GlobalSkillHandler, *serviceauth.Service, *globalSkillRepositoryStub) {
	t.Helper()
	repo := newGlobalSkillRepositoryStub()
	auth := newGlobalSkillTestAuth(t, map[string]bool{
		"admin@example.test":  true,
		"member@example.test": false,
	})
	handler := NewGlobalSkillHandler(serviceskills.NewGlobalService(repo, nil), auth)
	return handler, auth, repo
}

func globalSkillRequest(
	t *testing.T,
	auth *serviceauth.Service,
	email string,
	method string,
	target string,
	body string,
	contentType string,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if email != "" {
		request.AddCookie(&http.Cookie{
			Name:  serviceauth.SessionCookieName,
			Value: auth.SignSession(serviceauth.User{Email: email, Sub: "test"}),
		})
	}
	return request
}

func serveGlobalSkills(handler *GlobalSkillHandler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestGlobalSkillHandlerRequiresAdmin(t *testing.T) {
	handler, auth, _ := newGlobalSkillHandler(t)

	tests := []struct {
		name   string
		email  string
		method string
		target string
		want   int
	}{
		{name: "anonymous list", method: http.MethodGet, target: globalSkillsRoute, want: http.StatusUnauthorized},
		{name: "member list", email: "member@example.test", method: http.MethodGet, target: globalSkillsRoute, want: http.StatusForbidden},
		{name: "member create", email: "member@example.test", method: http.MethodPost, target: globalSkillsRoute, want: http.StatusForbidden},
		{name: "member read", email: "member@example.test", method: http.MethodGet, target: globalSkillsRoute + "/guard", want: http.StatusForbidden},
		{name: "member delete", email: "member@example.test", method: http.MethodDelete, target: globalSkillsRoute + "/guard", want: http.StatusForbidden},
		{name: "member import", email: "member@example.test", method: http.MethodPost, target: globalSkillsRoute + "/import", want: http.StatusForbidden},
		{name: "admin list", email: "admin@example.test", method: http.MethodGet, target: globalSkillsRoute, want: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := globalSkillRequest(t, auth, test.email, test.method, test.target, "{}", "application/json")
			if got := serveGlobalSkills(handler, request).Code; got != test.want {
				t.Fatalf("status = %d, want %d", got, test.want)
			}
		})
	}
}

func TestGlobalSkillHandlerCreateReadUpdateDelete(t *testing.T) {
	handler, auth, repo := newGlobalSkillHandler(t)

	body := `{"name":"guard","files":{"SKILL.md":` + jsonString(globalSkillManifest) + `},"alwaysOn":true}`
	response := serveGlobalSkills(handler, globalSkillRequest(
		t, auth, "admin@example.test", http.MethodPost, globalSkillsRoute, body, "application/json",
	))
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", response.Code, response.Body)
	}
	if !repo.records["guard"].AlwaysOn {
		t.Fatal("create did not persist the alwaysOn flag")
	}

	response = serveGlobalSkills(handler, globalSkillRequest(
		t, auth, "admin@example.test", http.MethodPost, globalSkillsRoute, body, "application/json",
	))
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", response.Code)
	}

	response = serveGlobalSkills(handler, globalSkillRequest(
		t, auth, "admin@example.test", http.MethodGet, globalSkillsRoute+"/guard", "", "",
	))
	if response.Code != http.StatusOK {
		t.Fatalf("read status = %d body = %s", response.Code, response.Body)
	}
	var read serviceskills.GlobalSkill
	if err := json.Unmarshal(response.Body.Bytes(), &read); err != nil {
		t.Fatalf("decode read: %v", err)
	}
	if read.Files["SKILL.md"] != globalSkillManifest || read.Description != "Guard rails." {
		t.Fatalf("read = %#v, want the stored manifest and its description", read)
	}

	response = serveGlobalSkills(handler, globalSkillRequest(
		t, auth, "admin@example.test", http.MethodPut, globalSkillsRoute+"/guard",
		`{"alwaysOn":false}`, "application/json",
	))
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", response.Code, response.Body)
	}
	if repo.records["guard"].AlwaysOn {
		t.Fatal("update did not clear the alwaysOn flag")
	}
	if len(repo.records["guard"].Files) != 1 {
		t.Fatal("a flag-only update must keep the stored files")
	}

	response = serveGlobalSkills(handler, globalSkillRequest(
		t, auth, "admin@example.test", http.MethodDelete, globalSkillsRoute+"/guard", "", "",
	))
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", response.Code, response.Body)
	}
	if _, ok := repo.records["guard"]; ok {
		t.Fatal("delete left the skill in the library")
	}

	response = serveGlobalSkills(handler, globalSkillRequest(
		t, auth, "admin@example.test", http.MethodGet, globalSkillsRoute+"/guard", "", "",
	))
	if response.Code != http.StatusNotFound {
		t.Fatalf("read after delete status = %d, want 404", response.Code)
	}
}

func TestGlobalSkillHandlerRejectsInvalidPayloads(t *testing.T) {
	handler, auth, _ := newGlobalSkillHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing manifest", body: `{"name":"guard","files":{"README.md":"x"}}`},
		{name: "invalid name", body: `{"name":"../escape","files":{"SKILL.md":"x"}}`},
		{name: "escaping path", body: `{"name":"guard","files":{"SKILL.md":"x","../evil":"y"}}`},
		{name: "not json", body: `nope`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := globalSkillRequest(
				t, auth, "admin@example.test", http.MethodPost, globalSkillsRoute, test.body, "application/json",
			)
			if got := serveGlobalSkills(handler, request).Code; got != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", got)
			}
		})
	}
}

func TestGlobalSkillHandlerAcceptsZipUpload(t *testing.T) {
	handler, auth, repo := newGlobalSkillHandler(t)

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"guard/SKILL.md":      globalSkillManifest,
		"guard/refs/rules.md": "rules",
	} {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	request := globalSkillRequest(
		t, auth, "admin@example.test", http.MethodPost, globalSkillsRoute,
		buffer.String(), "application/zip",
	)
	if response := serveGlobalSkills(handler, request); response.Code != http.StatusCreated {
		t.Fatalf("zip upload status = %d body = %s", response.Code, response.Body)
	}

	stored, ok := repo.records["guard"]
	if !ok {
		t.Fatalf("library = %v, want the archive root used as the skill name", repo.records)
	}
	if string(stored.Files["SKILL.md"]) != globalSkillManifest {
		t.Fatalf("stored manifest = %q", stored.Files["SKILL.md"])
	}
	if string(stored.Files["refs/rules.md"]) != "rules" {
		t.Fatalf("stored files = %v, want the nested support file", stored.FileNames)
	}
}

func TestGlobalSkillHandlerRejectsNestedPaths(t *testing.T) {
	handler, auth, _ := newGlobalSkillHandler(t)

	request := globalSkillRequest(
		t, auth, "admin@example.test", http.MethodGet, globalSkillsRoute+"/guard/SKILL.md", "", "",
	)
	if got := serveGlobalSkills(handler, request).Code; got != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", got)
	}
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
