package applications

import (
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func TestWebRouteRequiresProjectService(t *testing.T) {
	valid := svc.Application{
		ID: "editor", Name: "Editor", Version: "1", Scopes: []svc.Scope{svc.ScopeProject},
		Web:     &svc.ApplicationWeb{Port: 8400},
		Service: &svc.ApplicationService{Name: "editor", Command: []string{"/usr/bin/editor"}},
	}
	if err := validateApplication(valid); err != nil {
		t.Fatalf("valid web manifest: %v", err)
	}
	mismatched := valid
	web := *valid.Web
	web.Port = 80
	mismatched.Web = &web
	if err := validateApplication(mismatched); err == nil {
		t.Fatal("accepted privileged web port")
	}
	missingService := valid
	missingService.Service = nil
	if err := validateApplication(missingService); err == nil {
		t.Fatal("accepted web route without a service")
	}
	global := valid
	global.Scopes = []svc.Scope{svc.ScopeGlobal}
	if err := validateApplication(global); err == nil {
		t.Fatal("accepted global web route")
	}
}
