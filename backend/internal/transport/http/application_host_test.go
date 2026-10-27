package httptransport

import (
	"strings"
	"testing"
)

func TestApplicationHosts(t *testing.T) {
	for _, host := range []string{"abcdef123456.apps.remote.test", "apps.remote.test"} {
		if _, _, ok := ApplicationProject(host, "remote.test"); ok {
			t.Fatal("accepted removed origin", host)
		}
	}
	if ApplicationHost("abcdef123456", "", "remote.test") != "" {
		t.Fatal("accepted missing manifest subdomain")
	}
}

func TestProjectApplicationHosts(t *testing.T) {
	for _, label := range []string{"code", "editor-2"} {
		for _, slug := range []string{"gamerhead", "another-project"} {
			host := ApplicationHost(slug, label, "remote.test")
			if host != label+"--"+slug+".remote.test" {
				t.Fatal(host)
			}
			gotLabel, gotSlug, ok := ApplicationProject(host+":8443", "remote.test")
			if !ok || gotLabel != label || gotSlug != slug {
				t.Fatal(host, gotLabel, gotSlug)
			}
		}
	}
	for _, host := range []string{"code--project.remote.test.evil", "extra.code--project.remote.test", "code.-bad.remote.test", "code.project-.remote.test", "code..remote.test", "slug--3000.dev.remote.test", "slug.code.remote.test"} {
		if _, _, ok := ApplicationProject(host, "remote.test"); ok {
			t.Fatal(host)
		}
	}
}

func TestApplicationHostSeparatorAndLength(t *testing.T) {
	for _, slug := range []string{"code", "dev", "apps", "gamerhead", "other-project"} {
		host := ApplicationHost(slug, "code", "example.com")
		label, gotSlug, ok := ApplicationProject(strings.ToUpper(host)+".:443", "example.com")
		if !ok || label != "code" || gotSlug != slug {
			t.Fatal(host, label, gotSlug, ok)
		}
	}
	for _, host := range []string{"code.project.example.com", "code--a--b.example.com", "code---a.example.com", "--project.example.com", "code--.example.com", strings.Repeat("a", 58) + "--proj.example.com"} {
		if _, _, ok := ApplicationProject(host, "example.com"); ok {
			t.Fatal("accepted", host)
		}
	}
	if ApplicationHost("a--b", "code", "example.com") != "" {
		t.Fatal("accepted reserved separator")
	}
	if ApplicationHost("proj", strings.Repeat("a", 58), "example.com") != "" {
		t.Fatal("accepted oversized DNS label")
	}
	if ApplicationHost("proj", strings.Repeat("a", 57), "example.com") == "" {
		t.Fatal("rejected 63-character label")
	}
}
