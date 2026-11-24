package harness

import (
	"context"
	"testing"
)

type preparerWithoutInstructions struct{}

func (preparerWithoutInstructions) Prepare(context.Context, ProjectPreparationRequest, func(Event)) (PreparedProject, error) {
	return PreparedProject{}, nil
}

type preparerWithInstructions struct{ preparerWithoutInstructions }

func (preparerWithInstructions) Instructions() string { return "shared" }

func TestProjectInstructionsOnlyForProjectRunsWithASource(t *testing.T) {
	tests := []struct {
		name      string
		preparer  ProjectPreparer
		projectID string
		want      string
	}{
		{"project run", preparerWithInstructions{}, "p1", "shared"},
		{"host run", preparerWithInstructions{}, "", ""},
		{"no preparer", nil, "p1", ""},
		{"preparer without instructions", preparerWithoutInstructions{}, "p1", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ProjectInstructions(test.preparer, test.projectID); got != test.want {
				t.Fatalf("ProjectInstructions = %q, want %q", got, test.want)
			}
		})
	}
}
