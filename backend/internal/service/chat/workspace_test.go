package chat

import (
	"context"
	"errors"
	"testing"
)

type workspaceProjectResolver struct {
	roots map[ProjectID]string
	err   error
}

func (r workspaceProjectResolver) WorkspaceForProject(_ context.Context, id ProjectID) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	root, ok := r.roots[id]
	if !ok {
		return "", errors.New("project not found")
	}
	return root, nil
}

func TestTrustedWorkspaceRootIgnoresCallerControlledChatCwd(t *testing.T) {
	t.Parallel()
	const (
		projectRoot      = "/var/lib/remote/projects/allowed/workspace"
		otherProjectRoot = "/var/lib/remote/projects/other/workspace"
		hostRoot         = "/opt/remote.futrx"
	)
	service := New(
		nil,
		workspaceProjectResolver{roots: map[ProjectID]string{"allowed": projectRoot}},
		nil,
		nil,
		WithHostWorkspaceRoot(hostRoot),
	)

	tests := []struct {
		name string
		meta Meta
		want string
	}{
		{
			name: "project chat at its canonical root",
			meta: Meta{ProjectID: "allowed", Cwd: projectRoot},
			want: projectRoot,
		},
		{
			name: "project chat cannot select the host root",
			meta: Meta{ProjectID: "allowed", Cwd: "/"},
			want: projectRoot,
		},
		{
			name: "project chat cannot select another project",
			meta: Meta{ProjectID: "allowed", Cwd: otherProjectRoot},
			want: projectRoot,
		},
		{
			name: "loose chat uses the configured host workspace",
			meta: Meta{Cwd: hostRoot},
			want: hostRoot,
		},
		{
			name: "loose chat cannot select an arbitrary absolute root",
			meta: Meta{Cwd: "/etc"},
			want: hostRoot,
		},
		{
			name: "loose chat cannot select a project workspace",
			meta: Meta{Cwd: otherProjectRoot},
			want: hostRoot,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.TrustedWorkspaceRoot(context.Background(), test.meta)
			if err != nil {
				t.Fatalf("TrustedWorkspaceRoot: %v", err)
			}
			if got != test.want {
				t.Fatalf("root = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTrustedWorkspaceRootFailsClosedWithoutServerOwnedRoot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		service *Service
		meta    Meta
	}{
		{
			name:    "loose chat without configured host root",
			service: New(nil, nil, nil, nil),
			meta:    Meta{Cwd: "/browser/chosen"},
		},
		{
			name:    "loose chat with relative configured root",
			service: New(nil, nil, nil, nil, WithHostWorkspaceRoot("relative/workspace")),
			meta:    Meta{},
		},
		{
			name:    "project chat without project resolver",
			service: New(nil, nil, nil, nil, WithHostWorkspaceRoot("/trusted/host")),
			meta:    Meta{ProjectID: "project-1", Cwd: "/browser/chosen"},
		},
		{
			name: "project resolver returns relative root",
			service: New(
				nil,
				workspaceProjectResolver{roots: map[ProjectID]string{"project-1": "relative/workspace"}},
				nil,
				nil,
			),
			meta: Meta{ProjectID: "project-1", Cwd: "/browser/chosen"},
		},
		{
			name: "project resolution fails",
			service: New(
				nil,
				workspaceProjectResolver{err: errors.New("lookup failed")},
				nil,
				nil,
			),
			meta: Meta{ProjectID: "project-1", Cwd: "/browser/chosen"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.service.TrustedWorkspaceRoot(context.Background(), test.meta)
			if !errors.Is(err, ErrWorkspaceUnavailable) {
				t.Fatalf("error = %v, want ErrWorkspaceUnavailable", err)
			}
		})
	}
}

func TestTrustedWorkspaceRootPreservesExactServerOwnedRoot(t *testing.T) {
	t.Parallel()
	const (
		hostRoot    = "/srv/workspace/remote.futrx"
		projectRoot = "/srv/workspace/projects/allowed/workspace"
	)
	service := New(
		nil,
		workspaceProjectResolver{roots: map[ProjectID]string{"allowed": projectRoot}},
		nil,
		nil,
		WithHostWorkspaceRoot(hostRoot),
	)

	for _, test := range []struct {
		name string
		meta Meta
		want string
	}{
		{name: "host root nested beneath workspace directory", meta: Meta{}, want: hostRoot},
		{name: "project root nested beneath workspace directory", meta: Meta{ProjectID: "allowed"}, want: projectRoot},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.TrustedWorkspaceRoot(context.Background(), test.meta)
			if err != nil {
				t.Fatalf("TrustedWorkspaceRoot: %v", err)
			}
			if got != test.want {
				t.Fatalf("root = %q, want exact server-owned root %q", got, test.want)
			}
		})
	}
}
