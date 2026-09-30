package main

import (
	"flag"
	"reflect"
	"testing"
)

func testFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("endpoint", "", "")
	fs.String("region", "", "")
	fs.Bool("path-style", false, "")
	fs.Bool("read-only", false, "")
	return fs
}

func TestPermuteHoistsOptionsPastPositionalArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "options after positionals",
			in:   []string{"s3://bucket", "/mnt", "--endpoint", "http://x", "--path-style"},
			want: []string{"--endpoint", "http://x", "--path-style", "s3://bucket", "/mnt"},
		},
		{
			name: "attached values",
			in:   []string{"s3://bucket", "/mnt", "--endpoint=http://x"},
			want: []string{"--endpoint=http://x", "s3://bucket", "/mnt"},
		},
		{
			name: "already in order",
			in:   []string{"--read-only", "s3://bucket", "/mnt"},
			want: []string{"--read-only", "s3://bucket", "/mnt"},
		},
		{
			name: "interleaved",
			in:   []string{"--region", "eu-west-1", "s3://bucket", "--read-only", "/mnt"},
			want: []string{"--region", "eu-west-1", "--read-only", "s3://bucket", "/mnt"},
		},
		{
			name: "double dash ends option parsing",
			in:   []string{"s3://bucket", "--", "--not-an-option"},
			want: []string{"s3://bucket", "--not-an-option"},
		},
		{
			name: "a bool flag does not swallow the next word",
			in:   []string{"--path-style", "s3://bucket", "/mnt"},
			want: []string{"--path-style", "s3://bucket", "/mnt"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := permute(testFlagSet(), c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("permute(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

func TestPermutedArgsParseCorrectly(t *testing.T) {
	// The bug this guards against: flags after the positional arguments were
	// silently ignored, so a mount aimed at MinIO went to real AWS instead.
	fs := testFlagSet()
	args := []string{"s3://bucket", "/mnt", "--endpoint", "http://minio:9000", "--path-style"}
	if err := fs.Parse(permute(fs, args)); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := fs.Lookup("endpoint").Value.String(); got != "http://minio:9000" {
		t.Errorf("endpoint = %q; want the value given after the positional arguments", got)
	}
	if fs.Lookup("path-style").Value.String() != "true" {
		t.Error("path-style was not applied")
	}
	if got := fs.Args(); !reflect.DeepEqual(got, []string{"s3://bucket", "/mnt"}) {
		t.Errorf("positional args = %q", got)
	}
}

func TestSplitOptions(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"ro,allow-other", []string{"ro", "allow-other"}},
		{"cache-size=8G,uid=1000", []string{"cache-size=8G", "uid=1000"}},
		{`endpoint="http://a,b",ro`, []string{`endpoint=http://a,b`, "ro"}},
		{"", []string{""}},
	}
	for _, c := range cases {
		if got := splitOptions(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitOptions(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestMountOptionsMapOntoFlags(t *testing.T) {
	// Everything usable on the command line must also work from fstab via -o.
	m := newMountFlags()
	if err := m.applyOptions("ro,cache-size=2G,attr-mode=fast,_netdev,noatime"); err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	if !m.cfg.ReadOnly {
		t.Error("-o ro did not set read-only")
	}
	if m.cacheSize != "2G" {
		t.Errorf("cache-size = %q; want 2G", m.cacheSize)
	}
	if m.cfg.AttrMode != "fast" {
		t.Errorf("attr-mode = %q; want fast", m.cfg.AttrMode)
	}
	if err := m.applyOptions("no-such-option=1"); err == nil {
		t.Error("an unknown -o option should be reported, not ignored")
	}
}
