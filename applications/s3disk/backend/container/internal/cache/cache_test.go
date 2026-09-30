package cache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory stand-in for S3, so the cache can be exercised
// without a network or a server.
type fakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	gets    int
	puts    int
	ranges  [][2]int64
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (s *fakeStore) fetch(_ context.Context, key string, off, length int64, dst io.Writer) (int64, error) {
	s.mu.Lock()
	body, ok := s.objects[key]
	s.gets++
	s.ranges = append(s.ranges, [2]int64{off, length})
	s.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("no such object %q", key)
	}
	if off >= int64(len(body)) {
		return 0, nil
	}
	end := off + length
	if end > int64(len(body)) {
		end = int64(len(body))
	}
	n, err := dst.Write(body[off:end])
	return int64(n), err
}

func (s *fakeStore) upload(_ context.Context, key string, body io.ReadSeeker, size int64) (string, error) {
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	if int64(len(data)) != size {
		return "", fmt.Errorf("upload of %q: got %d bytes, expected %d", key, len(data), size)
	}
	s.mu.Lock()
	s.objects[key] = data
	s.puts++
	s.mu.Unlock()
	return fmt.Sprintf("etag-%d", len(data)), nil
}

func newTestCache(t *testing.T, store *fakeStore, blockSize int64) *Cache {
	t.Helper()
	c, err := New(Options{
		Dir:       t.TempDir(),
		BlockSize: blockSize,
		MaxBytes:  1 << 30,
		Fetch:     store.fetch,
		Upload:    store.upload,
		Log:       func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func readAll(t *testing.T, e *Entry) []byte {
	t.Helper()
	out := make([]byte, e.Size())
	if len(out) == 0 {
		return out
	}
	n, err := e.ReadAt(context.Background(), out, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt: %v", err)
	}
	return out[:n]
}

func TestWriteThenFlushStoresTheWholeObject(t *testing.T) {
	store := newFakeStore()
	c := newTestCache(t, store, 4096)
	ctx := context.Background()

	e, err := c.Create("file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.WriteAt(ctx, []byte("hello world"), 0); err != nil {
		t.Fatal(err)
	}
	if !e.Dirty() {
		t.Error("an entry with unsaved writes should be dirty")
	}
	if err := e.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if e.Dirty() {
		t.Error("an entry should be clean after a successful flush")
	}
	if got := string(store.objects["file.txt"]); got != "hello world" {
		t.Errorf("stored object = %q; want %q", got, "hello world")
	}
	if store.puts != 1 {
		t.Errorf("puts = %d; want 1", store.puts)
	}
	if err := e.Flush(ctx); err != nil || store.puts != 1 {
		t.Errorf("flushing a clean entry should not upload again (puts=%d, err=%v)", store.puts, err)
	}
}

func TestReadFaultsInOnlyTheBlocksItNeeds(t *testing.T) {
	const blockSize = 1024
	store := newFakeStore()
	body := bytes.Repeat([]byte("0123456789"), 1024) // 10 KiB, 10 blocks
	store.objects["big.bin"] = body
	c := newTestCache(t, store, blockSize)
	ctx := context.Background()

	e, err := c.Open("big.bin", int64(len(body)), "etag", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Unref()

	// Read 16 bytes from the middle: one block, not the whole object.
	buf := make([]byte, 16)
	if _, err := e.ReadAt(ctx, buf, 5000); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, body[5000:5016]) {
		t.Errorf("read %q; want %q", buf, body[5000:5016])
	}
	if store.gets != 1 {
		t.Fatalf("gets = %d; want a single ranged GET", store.gets)
	}
	if got := store.ranges[0]; got[1] > blockSize {
		t.Errorf("fetched %d bytes for a 16-byte read; want at most one %d-byte block", got[1], blockSize)
	}

	// The same block again must not go back to the store.
	if _, err := e.ReadAt(ctx, buf, 5020); err != nil {
		t.Fatal(err)
	}
	if store.gets != 1 {
		t.Errorf("gets = %d; a cached block should not be refetched", store.gets)
	}
}

func TestPartialWriteReadsModifiesWrites(t *testing.T) {
	store := newFakeStore()
	store.objects["doc"] = []byte("AAAABBBBCCCCDDDD")
	c := newTestCache(t, store, 4096)
	ctx := context.Background()

	e, err := c.Open("doc", 16, "etag", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Unref()

	// Overwrite four bytes in the middle; the rest must survive.
	if _, err := e.WriteAt(ctx, []byte("xxxx"), 4); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := string(store.objects["doc"]); got != "AAAAxxxxCCCCDDDD" {
		t.Errorf("stored = %q; want %q", got, "AAAAxxxxCCCCDDDD")
	}
}

func TestTruncateDownThenUpReadsZeros(t *testing.T) {
	store := newFakeStore()
	store.objects["doc"] = []byte("ABCDEFGHIJ")
	c := newTestCache(t, store, 4)
	ctx := context.Background()

	e, err := c.Open("doc", 10, "etag", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Unref()

	if err := e.Truncate(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if got := string(readAll(t, e)); got != "ABCD" {
		t.Fatalf("after truncate to 4: %q", got)
	}
	// Growing again must expose zeros, not the discarded tail from S3.
	if err := e.Truncate(ctx, 10); err != nil {
		t.Fatal(err)
	}
	want := append([]byte("ABCD"), make([]byte, 6)...)
	if got := readAll(t, e); !bytes.Equal(got, want) {
		t.Errorf("after regrowing: %q; want %q", got, want)
	}
	if err := e.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := store.objects["doc"]; !bytes.Equal(got, want) {
		t.Errorf("stored = %q; want %q", got, want)
	}
}

func TestSparseWritePastEndOfObject(t *testing.T) {
	store := newFakeStore()
	store.objects["doc"] = []byte("abc")
	c := newTestCache(t, store, 8)
	ctx := context.Background()

	e, err := c.Open("doc", 3, "etag", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Unref()

	if _, err := e.WriteAt(ctx, []byte("Z"), 20); err != nil {
		t.Fatal(err)
	}
	if err := e.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got := store.objects["doc"]
	want := append(append([]byte("abc"), make([]byte, 17)...), 'Z')
	if !bytes.Equal(got, want) {
		t.Errorf("stored %q (%d bytes); want %q (%d bytes)", got, len(got), want, len(want))
	}
}

func TestRenameMovesTheBackingFile(t *testing.T) {
	// A renamed entry must stop using the cache file named after its old key,
	// or a new file created at the old name would share the same bytes.
	store := newFakeStore()
	c := newTestCache(t, store, 4096)
	ctx := context.Background()

	src, err := c.Create("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.WriteAt(ctx, []byte("original contents"), 0); err != nil {
		t.Fatal(err)
	}
	if err := src.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c.Rename("a.txt", "b.txt")

	// Now write a different file at the old key.
	fresh, err := c.Create("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.WriteAt(ctx, []byte("brand new"), 0); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	// The renamed entry must still hold its own bytes, not the new file's.
	if got := string(readAll(t, src)); got != "original contents" {
		t.Errorf("renamed entry now reads %q; want %q", got, "original contents")
	}
	if got := string(readAll(t, fresh)); got != "brand new" {
		t.Errorf("new entry at the old key reads %q; want %q", got, "brand new")
	}
	if got := string(store.objects["a.txt"]); got != "brand new" {
		t.Errorf("a.txt = %q; want %q", got, "brand new")
	}
	// Uploading the renamed entry must target its new key. (Copying the object
	// in S3 is the filesystem layer's job; the cache only rebinds the key.)
	if _, err := src.WriteAt(ctx, []byte("!"), 17); err != nil {
		t.Fatal(err)
	}
	if err := src.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := string(store.objects["b.txt"]); got != "original contents!" {
		t.Errorf("b.txt = %q; want %q", got, "original contents!")
	}
}

func TestDiskAccountingMatchesReality(t *testing.T) {
	store := newFakeStore()
	c := newTestCache(t, store, 4096)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		e, err := c.Create(fmt.Sprintf("f%d.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.WriteAt(ctx, []byte("tiny"), 0); err != nil {
			t.Fatal(err)
		}
		if err := e.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		e.Unref()
	}
	if got := c.Stats().Bytes; got != 20*4 {
		t.Errorf("cache bytes = %d; want %d (a small file must not be charged a whole block)", got, 20*4)
	}

	// Deleting entries must return the space, never overshoot into negatives.
	for i := 0; i < 20; i++ {
		c.Remove(fmt.Sprintf("f%d.txt", i))
	}
	if got := c.Stats().Bytes; got != 0 {
		t.Errorf("cache bytes after removing everything = %d; want 0", got)
	}
}

func TestEvictionStaysWithinBudget(t *testing.T) {
	store := newFakeStore()
	c, err := New(Options{
		Dir:       t.TempDir(),
		BlockSize: 1024,
		MaxBytes:  16 * 1024,
		Fetch:     store.fetch,
		Upload:    store.upload,
		Log:       func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	ctx := context.Background()

	payload := bytes.Repeat([]byte("x"), 4096)
	for i := 0; i < 40; i++ {
		key := fmt.Sprintf("f%d.bin", i)
		e, err := c.Create(key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.WriteAt(ctx, payload, 0); err != nil {
			t.Fatal(err)
		}
		if err := e.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		e.Unref()
	}
	// Eviction runs asynchronously; give it a moment to catch up.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && c.Stats().Bytes > 16*1024 {
		time.Sleep(20 * time.Millisecond)
	}
	stats := c.Stats()
	if stats.Bytes > 16*1024 {
		t.Errorf("cache holds %d bytes; budget is %d", stats.Bytes, 16*1024)
	}
	if stats.Bytes < 0 {
		t.Errorf("cache byte count went negative: %d", stats.Bytes)
	}
	if stats.Evictions == 0 {
		t.Error("expected some evictions once past the budget")
	}
	// Evicted data must still be readable: it is in the store.
	e, err := c.Open("f0.bin", 4096, "etag-4096", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Unref()
	if got := readAll(t, e); !bytes.Equal(got, payload) {
		t.Error("an evicted file did not read back correctly from the store")
	}
}

func TestConcurrentReadersAndWriters(t *testing.T) {
	store := newFakeStore()
	body := make([]byte, 64*1024)
	rng := rand.New(rand.NewSource(1))
	rng.Read(body)
	store.objects["shared.bin"] = body
	c := newTestCache(t, store, 4096)
	ctx := context.Background()

	e, err := c.Open("shared.bin", int64(len(body)), "etag", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Unref()

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(seed int) { // readers
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed)))
			buf := make([]byte, 512)
			for j := 0; j < 50; j++ {
				off := int64(r.Intn(len(body) - 512))
				if _, err := e.ReadAt(ctx, buf, off); err != nil && err != io.EOF {
					errs <- err
					return
				}
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(seed int) { // writers, each to its own region
			defer wg.Done()
			region := int64(seed) * 8192
			for j := 0; j < 50; j++ {
				if _, err := e.WriteAt(ctx, []byte("written"), region+int64(j)); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent access: %v", err)
	}
	if err := e.Flush(ctx); err != nil {
		t.Fatalf("flush after concurrent access: %v", err)
	}
	if got := int64(len(store.objects["shared.bin"])); got != int64(len(body)) {
		t.Errorf("object length changed to %d; want %d", got, len(body))
	}
}

func TestCacheSurvivesRestartAndRecoversDirtyData(t *testing.T) {
	store := newFakeStore()
	dir := t.TempDir()
	ctx := context.Background()

	open := func() *Cache {
		c, err := New(Options{
			Dir: dir, BlockSize: 4096, MaxBytes: 1 << 30, Persist: true,
			Fetch: store.fetch, Upload: store.upload, Log: func(string, ...any) {},
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	c := open()
	e, err := c.Create("unsaved.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.WriteAt(ctx, []byte("never closed"), 0); err != nil {
		t.Fatal(err)
	}
	// Persist the index without flushing, mimicking a checkpoint before a kill.
	if err := c.saveIndex(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.objects["unsaved.txt"]; ok {
		t.Fatal("the object should not be in the store yet")
	}

	// A new cache over the same directory must find and upload the lost write.
	c2 := open()
	defer c2.Close(ctx)
	n, err := c2.Recover(ctx, func(string, ...any) {})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if n != 1 {
		t.Errorf("recovered %d files; want 1", n)
	}
	if got := string(store.objects["unsaved.txt"]); got != "never closed" {
		t.Errorf("recovered object = %q; want %q", got, "never closed")
	}
}
