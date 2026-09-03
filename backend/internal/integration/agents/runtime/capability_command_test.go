package runtime

import (
	"slices"
	"testing"
)

func TestMergeEnvironmentReplacesInheritedValues(t *testing.T) {
	merged := mergeEnvironment(
		[]string{"HOME=/old", "PATH=/bin", "OPENAI_API_KEY=secret"},
		[]string{"HOME=/provider", "OPENAI_API_KEY="},
	)

	want := []string{"PATH=/bin", "HOME=/provider", "OPENAI_API_KEY="}
	if !slices.Equal(merged, want) {
		t.Fatalf("mergeEnvironment() = %v, want %v", merged, want)
	}
}
