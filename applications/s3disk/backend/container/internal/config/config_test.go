package config

import (
	"strings"
	"testing"
)

func TestParseS3URL(t *testing.T) {
	cases := []struct {
		in             string
		bucket, prefix string
		wantErr        bool
	}{
		{in: "s3://bucket", bucket: "bucket"},
		{in: "s3://bucket/", bucket: "bucket"},
		{in: "s3://bucket/prefix", bucket: "bucket", prefix: "prefix/"},
		{in: "s3://bucket/a/b/c", bucket: "bucket", prefix: "a/b/c/"},
		{in: "s3://bucket/a/b/c/", bucket: "bucket", prefix: "a/b/c/"},
		{in: "bucket", bucket: "bucket"},
		{in: "bucket:prefix", bucket: "bucket", prefix: "prefix/"}, // s3fs spelling
		{in: "bucket/prefix", bucket: "bucket", prefix: "prefix/"}, // goofys spelling
		{in: "bucket:a/b", bucket: "bucket", prefix: "a/b/"},
		{in: "", wantErr: true},
		{in: "s3://", wantErr: true},
		{in: "s3:///prefix", wantErr: true},
		// A provider endpoint pasted into the bucket field. This used to parse
		// as the bucket "https", producing a mount aimed at the wrong service.
		{in: "https://de-s3.storage.bunnycdn.com", wantErr: true},
		{in: "http://minio:9000", wantErr: true},
		{in: "https://s3.us-west-2.amazonaws.com/my-bucket", wantErr: true},
	}
	for _, c := range cases {
		bucket, prefix, err := ParseS3URL(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseS3URL(%q): expected an error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseS3URL(%q): %v", c.in, err)
			continue
		}
		if bucket != c.bucket || prefix != c.prefix {
			t.Errorf("ParseS3URL(%q) = %q, %q; want %q, %q", c.in, bucket, prefix, c.bucket, c.prefix)
		}
	}
}

// The message has to name the mistake, because the failure it prevents shows
// up much later and somewhere else — as an auth error against the wrong host.
func TestParseS3URLExplainsAnEndpointInTheBucketField(t *testing.T) {
	_, _, err := ParseS3URL("https://de-s3.storage.bunnycdn.com")
	if err == nil {
		t.Fatal("expected an error for an endpoint URL")
	}
	for _, want := range []string{"URL", "endpoint", "s3://bucket"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"0": 0, "512": 512, "1K": 1 << 10, "8k": 8 << 10,
		"4M": 4 << 20, "8G": 8 << 30, "2T": 2 << 40,
		"8Gi": 8 << 30, "8GiB": 8 << 30, "1024B": 1024, "1.5M": 1572864,
	}
	for in, want := range cases {
		got, err := ParseSize(in)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSize(%q) = %d; want %d", in, got, want)
		}
	}
	for _, in := range []string{"", "abc", "12X3", "-5M"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q): expected an error", in)
		}
	}
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]uint32{"0644": 0644, "755": 0755, "0777": 0777, "600": 0600} {
		got, err := ParseMode(in)
		if err != nil || got != want {
			t.Errorf("ParseMode(%q) = %o, %v; want %o", in, got, err, want)
		}
	}
	if _, err := ParseMode("999"); err == nil {
		t.Error("ParseMode(\"999\"): expected an error for a non-octal value")
	}
}

// The mount's flag defaults are rendered from these values, so what a person
// reads in `s3disk mount -h` is whichever number Default holds. Formatting has
// to land on the spelling ParseSize accepts, and on the same one it did when
// the strings were written out by hand.
func TestDefaultsRenderAsTheirWrittenForm(t *testing.T) {
	cfg := Default()
	sizes := []struct {
		name string
		got  string
		want string
	}{
		{"cache-size", FormatSize(cfg.CacheSize), "8G"},
		{"block-size", FormatSize(cfg.BlockSize), "4M"},
		{"readahead", FormatSize(cfg.Readahead), "16M"},
		{"multipart-threshold", FormatSize(cfg.MultipartThreshold), "16M"},
		{"part-size", FormatSize(cfg.PartSize), "16M"},
	}
	for _, c := range sizes {
		if c.got != c.want {
			t.Errorf("%s default = %q, want %q", c.name, c.got, c.want)
		}
		n, err := ParseSize(c.got)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", c.got, err)
		} else if want, _ := ParseSize(c.want); n != want {
			t.Errorf("%s round trip = %d, want %d", c.name, n, want)
		}
	}
	if got := FormatMode(cfg.FileMode); got != "0644" {
		t.Errorf("file-mode default = %q, want %q", got, "0644")
	}
	if got := FormatMode(cfg.DirMode); got != "0755" {
		t.Errorf("dir-mode default = %q, want %q", got, "0755")
	}
	// A size with no exact unit stays a byte count rather than becoming a
	// rounded one, since the value is what the mount will use.
	if got := FormatSize(1234); got != "1234" {
		t.Errorf("FormatSize(1234) = %q, want %q", got, "1234")
	}
}

func TestValidate(t *testing.T) {
	base := func() *Config {
		c := Default()
		c.Bucket, c.Mountpoint = "b", "/mnt"
		return c
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("a default config should validate: %v", err)
	}

	c := base()
	c.Bucket = ""
	if err := c.Validate(); err == nil {
		t.Error("expected an error when no bucket is set")
	}

	c = base()
	c.AttrMode = "sideways"
	if err := c.Validate(); err == nil {
		t.Error("expected an error for an unknown attr mode")
	}

	c = base()
	c.PartSize = 1 << 20 // below S3's 5 MiB minimum
	if err := c.Validate(); err == nil {
		t.Error("expected an error for a part size below the S3 minimum")
	}

	c = base()
	if err := c.Validate(); err != nil || c.CacheDir == "" {
		t.Errorf("Validate should derive a cache directory, got %q (%v)", c.CacheDir, err)
	}
}
