package s3fs

import (
	"syscall"
	"testing"
	"time"

	"s3disk/internal/config"
	"s3disk/internal/s3io"
)

func testFS() *FS {
	cfg := config.Default()
	cfg.UID, cfg.GID = 1000, 1000
	cfg.FileMode, cfg.DirMode = 0644, 0755
	return &FS{cfg: cfg, attrs: newAttrCache(cfg), started: time.Now()}
}

func TestAttrFallsBackToMountDefaults(t *testing.T) {
	f := testFS()
	obj := &s3io.Object{Key: "a.txt", Size: 42, Modified: time.Unix(1700000000, 0)}

	a := f.attrFromObject(obj, false)
	if a.Mode != syscall.S_IFREG|0644 {
		t.Errorf("mode = %o; want %o", a.Mode, syscall.S_IFREG|0644)
	}
	if a.Uid != 1000 || a.Gid != 1000 {
		t.Errorf("ownership = %d:%d; want 1000:1000", a.Uid, a.Gid)
	}
	if a.Size != 42 {
		t.Errorf("size = %d; want 42", a.Size)
	}
	if !a.Mtime.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("mtime = %v; want the object's LastModified", a.Mtime)
	}
	if a.Atime.IsZero() || a.Ctime.IsZero() {
		t.Error("atime and ctime should fall back to mtime, not stay zero")
	}

	d := f.attrFromObject(&s3io.Object{Key: "dir/"}, true)
	if !d.IsDir() || d.Mode != syscall.S_IFDIR|0755 {
		t.Errorf("directory mode = %o; want %o", d.Mode, syscall.S_IFDIR|0755)
	}
}

func TestPosixMetadataRoundTrip(t *testing.T) {
	f := testFS()
	original := &Attr{
		Mode:  syscall.S_IFREG | 0751,
		Size:  128,
		Uid:   4242,
		Gid:   4343,
		Mtime: time.Unix(1600000000, 0),
		Atime: time.Unix(1600000001, 0),
		Ctime: time.Unix(1600000002, 0),
	}
	meta := metaFor(original)

	// s3fs-fuse uses exactly these keys; staying compatible keeps buckets
	// readable by both tools.
	for _, k := range []string{"mode", "uid", "gid", "mtime", "atime", "ctime"} {
		if _, ok := meta[k]; !ok {
			t.Errorf("metadata is missing the %q key", k)
		}
	}

	back := f.attrFromObject(&s3io.Object{Key: "f", Size: 128, Meta: meta}, false)
	if back.Mode != original.Mode {
		t.Errorf("mode round trip: %o -> %o", original.Mode, back.Mode)
	}
	if back.Uid != original.Uid || back.Gid != original.Gid {
		t.Errorf("ownership round trip: %d:%d -> %d:%d", original.Uid, original.Gid, back.Uid, back.Gid)
	}
	if !back.Mtime.Equal(original.Mtime) {
		t.Errorf("mtime round trip: %v -> %v", original.Mtime, back.Mtime)
	}
}

func TestSymlinkAndDirectoryModesSurvive(t *testing.T) {
	f := testFS()

	link := f.attrFromObject(&s3io.Object{
		Key: "l", Size: 7,
		Meta: map[string]string{"mode": itoa(syscall.S_IFLNK | 0777)},
	}, false)
	if !link.IsLink() {
		t.Errorf("mode %o was not recognised as a symlink", link.Mode)
	}

	// A directory marker carries S_IFDIR in its stored mode.
	dir := f.attrFromObject(&s3io.Object{
		Key: "d/", Meta: map[string]string{"mode": itoa(syscall.S_IFDIR | 0700)},
	}, true)
	if !dir.IsDir() || dir.Mode&07777 != 0700 {
		t.Errorf("directory mode = %o; want %o", dir.Mode, syscall.S_IFDIR|0700)
	}
}

func TestBarePermissionMetadataKeepsFileType(t *testing.T) {
	// Some writers store just the permission bits, with no file-type bits.
	f := testFS()
	a := f.attrFromObject(&s3io.Object{Key: "f", Meta: map[string]string{"mode": "493"}}, false) // 0755
	if a.Mode != syscall.S_IFREG|0755 {
		t.Errorf("mode = %o; want %o", a.Mode, syscall.S_IFREG|0755)
	}
}

func TestParseTimeVariants(t *testing.T) {
	if got, ok := parseTime("1600000000"); !ok || !got.Equal(time.Unix(1600000000, 0)) {
		t.Errorf("whole seconds: %v %v", got, ok)
	}
	if got, ok := parseTime("1600000000.500000000"); !ok || got.Unix() != 1600000000 {
		t.Errorf("fractional seconds: %v %v", got, ok)
	}
	for _, in := range []string{"", "not-a-time", "0", "-5"} {
		if _, ok := parseTime(in); ok {
			t.Errorf("parseTime(%q) should be rejected", in)
		}
	}
}

func TestAttrCacheExpiryAndStickiness(t *testing.T) {
	cfg := config.Default()
	cfg.StatTTL = 30 * time.Millisecond
	cfg.NegativeTTL = 30 * time.Millisecond
	c := newAttrCache(cfg)

	c.put("a", &Attr{Size: 1})
	if _, _, ok := c.get("a"); !ok {
		t.Fatal("a freshly cached attribute should be readable")
	}
	c.putNegative("gone")
	if _, negative, ok := c.get("gone"); !ok || !negative {
		t.Fatal("negative entries should be cached")
	}

	// A locally created file is authoritative until it is uploaded, so it must
	// not expire on a timer.
	c.putSticky("pending", &Attr{Size: 5})

	time.Sleep(60 * time.Millisecond)
	if _, _, ok := c.get("a"); ok {
		t.Error("a normal entry should expire")
	}
	if _, _, ok := c.get("gone"); ok {
		t.Error("a negative entry should expire")
	}
	if _, _, ok := c.get("pending"); !ok {
		t.Error("a sticky entry must not expire while the write is unsaved")
	}

	// Once uploaded it becomes an ordinary, expiring entry.
	c.unstick("pending", &Attr{Size: 5})
	time.Sleep(60 * time.Millisecond)
	if _, _, ok := c.get("pending"); ok {
		t.Error("an unstuck entry should expire like any other")
	}
}

func TestAttrCachePrefixInvalidation(t *testing.T) {
	c := newAttrCache(config.Default())
	for _, p := range []string{"dir", "dir/a", "dir/b/c", "dirty", "other"} {
		c.put(p, &Attr{})
	}
	c.invalidatePrefix("dir")
	for _, p := range []string{"dir", "dir/a", "dir/b/c"} {
		if _, _, ok := c.get(p); ok {
			t.Errorf("%q should have been invalidated with its subtree", p)
		}
	}
	// "dirty" merely shares a prefix string; it is not inside "dir".
	if _, _, ok := c.get("dirty"); !ok {
		t.Error(`"dirty" is not under "dir/" and must survive`)
	}
	if _, _, ok := c.get("other"); !ok {
		t.Error(`"other" must survive`)
	}
}

func TestContentTypeFromExtension(t *testing.T) {
	for name, want := range map[string]string{
		"a.json": "application/json",
		"b.bin":  "application/octet-stream",
		"noext":  "application/octet-stream",
	} {
		if got := contentTypeFor(name); got != want && !(want == "application/json" && got[:16] == "application/json") {
			t.Errorf("contentTypeFor(%q) = %q; want %q", name, got, want)
		}
	}
}

func itoa(v uint32) string {
	digits := ""
	if v == 0 {
		return "0"
	}
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
