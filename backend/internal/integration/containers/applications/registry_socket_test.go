package applications

import (
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"testing"
)

func TestSocketProxyValidation(t *testing.T) {
	cases := []struct {
		name  string
		proxy svc.SocketProxy
		port  int
		valid bool
	}{
		{"valid", svc.SocketProxy{ListenPort: 8400, TargetPort: 8401, IdleSeconds: 600, ReadyPath: "/healthz"}, 8400, true},
		{"privileged listener", svc.SocketProxy{ListenPort: 80, TargetPort: 8401, IdleSeconds: 600}, 0, false},
		{"invalid target", svc.SocketProxy{ListenPort: 8400, TargetPort: 65536, IdleSeconds: 600}, 0, false},
		{"same ports", svc.SocketProxy{ListenPort: 8400, TargetPort: 8400, IdleSeconds: 600}, 0, false},
		{"zero timeout", svc.SocketProxy{ListenPort: 8400, TargetPort: 8401}, 0, false},
		{"negative timeout", svc.SocketProxy{ListenPort: 8400, TargetPort: 8401, IdleSeconds: -1}, 0, false},
		{"mismatched exposed port", svc.SocketProxy{ListenPort: 8400, TargetPort: 8401, IdleSeconds: 600}, 8402, false},
		{"invalid readiness", svc.SocketProxy{ListenPort: 8400, TargetPort: 8401, IdleSeconds: 600, ReadyPath: "/health?token=x"}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := svc.Application{Service: &svc.ApplicationService{Name: "fixture", Command: []string{"/usr/bin/fixture"}, SocketProxy: &tc.proxy}, Port: svc.Port{Internal: tc.port}}
			err := validateService(app)
			if (err == nil) != tc.valid {
				t.Fatalf("validateService = %v, want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestSocketProxyMatchesProjectWebPort(t *testing.T) {
	app := svc.Application{
		ID: "editor", Name: "Editor", Version: "1", Scopes: []svc.Scope{svc.ScopeProject},
		Web: &svc.ApplicationWeb{Port: 8400},
		Service: &svc.ApplicationService{
			Name: "editor", Command: []string{"/usr/bin/editor"},
			SocketProxy: &svc.SocketProxy{ListenPort: 8400, TargetPort: 8401, IdleSeconds: 600},
		},
	}
	if err := validateApplication(app); err != nil {
		t.Fatalf("matching web listener: %v", err)
	}
	app.Web.Port = 8401
	if err := validateApplication(app); err == nil {
		t.Fatal("accepted web route that bypasses the activation listener")
	}
}
