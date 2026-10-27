package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationRequiresEveryDeclaredValue(t *testing.T) {
	_, err := configurationFromEnv(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "HELLO_GREETING_B64") {
		t.Fatalf("error = %v, want missing greeting", err)
	}
}

func TestReadsProvisionedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provisioned-version")
	if err := os.WriteFile(path, []byte("10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	version, err := readProvisionedVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if version != "10" {
		t.Fatalf("version = %q, want 10", version)
	}
}

func TestProvisionedVersionMustExistAndNotBeEmpty(t *testing.T) {
	if _, err := readProvisionedVersion(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing provisioning metadata was accepted")
	}

	path := filepath.Join(t.TempDir(), "provisioned-version")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readProvisionedVersion(path); err == nil {
		t.Fatal("empty provisioning metadata was accepted")
	}
}
