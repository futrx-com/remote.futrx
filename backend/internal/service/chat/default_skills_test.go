package chat

import (
	"context"
	"errors"
	"testing"
)

// captureRepository records the metadata Create receives; the remaining
// Repository methods are unused by these tests.
type captureRepository struct {
	created Meta
}

func (r *captureRepository) List(context.Context) ([]Meta, error) { return nil, nil }

func (r *captureRepository) Create(_ context.Context, meta Meta) (Meta, error) {
	meta.ID = "chat-1"
	r.created = meta
	return meta, nil
}

func (r *captureRepository) Get(context.Context, ID) (Meta, error) { return Meta{}, ErrNotFound }

func (r *captureRepository) Update(context.Context, ID, func(*Meta)) (Meta, error) {
	return Meta{}, ErrNotFound
}

func (r *captureRepository) Delete(context.Context, ID) error { return nil }

func (r *captureRepository) ReadEvents(context.Context, ID) ([]Event, error) { return nil, nil }

func (r *captureRepository) ReadEventsPage(context.Context, ID, EventPageQuery) (EventPage, error) {
	return EventPage{}, nil
}

func (r *captureRepository) ReadEventsAfter(context.Context, ID, int64) ([]Event, error) {
	return nil, nil
}

func (r *captureRepository) AppendEvent(_ context.Context, _ ID, ev Event) (Event, error) {
	return ev, nil
}

func (r *captureRepository) TruncateEventsBefore(context.Context, ID, int64) ([]Event, error) {
	return nil, nil
}

type stubProjectResolver struct{}

func (stubProjectResolver) WorkspaceForProject(context.Context, ProjectID) (string, error) {
	return "/var/lib/remote/projects/demo/workspace", nil
}

type stubDefaultSkills struct {
	skills []SkillRef
	err    error
	calls  int
}

func (s *stubDefaultSkills) DefaultSkills(
	_ context.Context,
	_ ProjectID,
	provider Provider,
) ([]SkillRef, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]SkillRef, 0, len(s.skills))
	for _, skill := range s.skills {
		skill.Provider = provider
		out = append(out, skill)
	}
	return out, nil
}

func TestCreateAppliesAlwaysOnGlobalSkills(t *testing.T) {
	alwaysOn := SkillRef{Name: "Code Review Guard", Command: "code-review-guard", Source: "global"}

	tests := []struct {
		name         string
		input        CreateInput
		resolver     *stubDefaultSkills
		wantCommands []string
		wantCalls    int
	}{
		{
			name:         "project chat gets the always-on skill",
			input:        CreateInput{ProjectID: "p1", Provider: ProviderClaude},
			resolver:     &stubDefaultSkills{skills: []SkillRef{alwaysOn}},
			wantCommands: []string{"code-review-guard"},
			wantCalls:    1,
		},
		{
			name: "explicit selection is preserved and not duplicated",
			input: CreateInput{
				ProjectID: "p1",
				Provider:  ProviderClaude,
				SelectedSkills: []SkillRef{
					{Name: "Browser", Command: "browser", Source: "project"},
					alwaysOn,
				},
			},
			resolver:     &stubDefaultSkills{skills: []SkillRef{alwaysOn}},
			wantCommands: []string{"browser", "code-review-guard"},
			wantCalls:    1,
		},
		{
			name:         "loose chat never consults the library",
			input:        CreateInput{Provider: ProviderClaude},
			resolver:     &stubDefaultSkills{skills: []SkillRef{alwaysOn}},
			wantCommands: nil,
			wantCalls:    0,
		},
		{
			name:         "a failing library leaves the chat unchanged",
			input:        CreateInput{ProjectID: "p1", Provider: ProviderClaude},
			resolver:     &stubDefaultSkills{err: errors.New("library unavailable")},
			wantCommands: nil,
			wantCalls:    1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &captureRepository{}
			service := New(repo, stubProjectResolver{}, nil, nil, WithDefaultSkills(test.resolver))

			created, err := service.Create(context.Background(), test.input)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if test.resolver.calls != test.wantCalls {
				t.Fatalf("resolver calls = %d, want %d", test.resolver.calls, test.wantCalls)
			}

			var commands []string
			for _, skill := range created.SelectedSkills {
				commands = append(commands, skill.Command)
				if skill.Provider != ProviderClaude {
					t.Fatalf("skill %q provider = %q, want the chat provider", skill.Command, skill.Provider)
				}
			}
			if len(commands) != len(test.wantCommands) {
				t.Fatalf("selected skills = %v, want %v", commands, test.wantCommands)
			}
			for index, want := range test.wantCommands {
				if commands[index] != want {
					t.Fatalf("selected skills = %v, want %v", commands, test.wantCommands)
				}
			}
		})
	}
}

func TestCreateWithoutADefaultSkillResolver(t *testing.T) {
	repo := &captureRepository{}
	service := New(repo, stubProjectResolver{}, nil, nil)

	created, err := service.Create(context.Background(), CreateInput{ProjectID: "p1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.SelectedSkills) != 0 {
		t.Fatalf("selected skills = %#v, want none", created.SelectedSkills)
	}
}
