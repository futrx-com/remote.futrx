package api

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func TestTargetRequiresManifestMetadataAndResolvedDefaults(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*applications.Instance)
	}{
		{"application id", func(instance *applications.Instance) { instance.ApplicationID = "" }},
		{"service", func(instance *applications.Instance) { instance.Service = "" }},
		{"mountpoint", func(instance *applications.Instance) { delete(instance.Env, "S3DISK_MOUNTPOINT") }},
		{"uploads directory", func(instance *applications.Instance) { delete(instance.Env, "S3DISK_UPLOADS_DIR") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			instance := testInstance()
			test.mutate(&instance)
			if _, err := newBackendTarget(instance); err == nil {
				t.Fatal("missing manifest-owned value was accepted")
			}
		})
	}
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

func TestUploadsDirIsAPlainRelativePath(t *testing.T) {
	for input, want := range map[string]string{"uploads": "uploads", ".": "", "/attachments/": "attachments", "a/b": "a/b"} {
		got, err := uploadsDirOf(input)
		if err != nil || got != want {
			t.Fatalf("uploadsDirOf(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "..", "a/../..", "-flag", "a/-flag", "a//b"} {
		if _, err := uploadsDirOf(input); err == nil {
			t.Fatalf("uploadsDirOf(%q) was accepted", input)
		}
	}
}
