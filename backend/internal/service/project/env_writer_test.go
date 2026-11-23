package project

import (
	"strings"
	"testing"
)

func TestMergeEnvFileKeepsProjectLines(t *testing.T) {
	existing := "APP_NAME=shop\nDB_HOST=127.0.0.1\n"
	got := mergeEnvFile(existing, []Secret{{Key: "API_TOKEN", Value: "abc"}})
	want := "APP_NAME=shop\nDB_HOST=127.0.0.1\n\n" +
		envBlockStart + "\nAPI_TOKEN=\"abc\"\n" + envBlockEnd + "\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeEnvFileReplacesOnlyTheBlock(t *testing.T) {
	first := mergeEnvFile("APP_NAME=shop\n", []Secret{{Key: "A", Value: "1"}, {Key: "B", Value: "2"}})
	edited := strings.Replace(first, "APP_NAME=shop\n", "APP_NAME=shop\nDEBUG=true\n", 1)
	got := mergeEnvFile(edited, []Secret{{Key: "A", Value: "3"}})
	want := "APP_NAME=shop\nDEBUG=true\n\n" + envBlockStart + "\nA=\"3\"\n" + envBlockEnd + "\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeEnvFileIsStable(t *testing.T) {
	secrets := []Secret{{Key: "A", Value: "1"}}
	once := mergeEnvFile("APP_NAME=shop\n", secrets)
	if twice := mergeEnvFile(once, secrets); twice != once {
		t.Fatalf("second merge changed the file:\n%s\nfirst:\n%s", twice, once)
	}
}

func TestMergeEnvFileWithoutSecretsDropsOnlyTheBlock(t *testing.T) {
	withBlock := mergeEnvFile("APP_NAME=shop\n", []Secret{{Key: "A", Value: "1"}})
	if got := mergeEnvFile(withBlock, nil); got != "APP_NAME=shop\n" {
		t.Fatalf("got %q, want the project lines only", got)
	}
	onlyBlock := mergeEnvFile("", []Secret{{Key: "A", Value: "1"}})
	if got := mergeEnvFile(onlyBlock, nil); got != "" {
		t.Fatalf("got %q, want empty so the file is removed", got)
	}
}

func TestMergeEnvFileReplacesLegacyGeneratedFile(t *testing.T) {
	legacy := legacyEnvHeader + "# Edits here will be overwritten on the next secret change.\nA=\"1\"\n"
	got := mergeEnvFile(legacy, []Secret{{Key: "A", Value: "2"}})
	want := envBlockStart + "\nA=\"2\"\n" + envBlockEnd + "\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeEnvFileHandlesCRLFAndMissingNewline(t *testing.T) {
	got := mergeEnvFile("APP_NAME=shop\r\nDEBUG=true", []Secret{{Key: "A", Value: "1"}})
	want := "APP_NAME=shop\r\nDEBUG=true\n\n" + envBlockStart + "\nA=\"1\"\n" + envBlockEnd + "\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if again := mergeEnvFile(got, []Secret{{Key: "A", Value: "1"}}); again != got {
		t.Fatalf("second merge changed the file: %q", again)
	}
}
