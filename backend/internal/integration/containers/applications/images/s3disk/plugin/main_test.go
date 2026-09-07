package main

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

func testBackend(t *testing.T) *backend {
	t.Helper()
	b := newBackend()
	if err := b.Init(appplugin.Instance{Scope: "project", ContainerName: "project-test", Env: map[string]string{"AWS_SECRET_ACCESS_KEY": "private-value"}}); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMountFlagIsMatchedAsAWholeWord(t *testing.T) {
	for args, want := range map[string]bool{
		"--exclusive --async-writeback": true,
		"--async-writeback=true":        true,
		"--no-async-writeback":          false,
		"--exclusive":                   false,
		"":                              false,
	} {
		if got := hasMountFlag(args, "--async-writeback"); got != want {
			t.Fatalf("hasMountFlag(%q) = %v; want %v", args, got, want)
		}
	}
}
