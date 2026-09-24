package workspace

import (
	"archive/zip"
	"container/heap"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	maxSearchVisits        = 200000
	maxArchiveSourceBytes  = int64(1 << 30)
	maxArchiveEntries      = 200000
	directoryReadBatchSize = 256
)

var (
	errOutsideWorkspace = errors.New("path escapes workspace")
	errNotRegularFile   = errors.New("not a regular file")
)

type archiveLimits struct {
	maxSourceBytes int64
	maxEntries     int
}

var defaultArchiveLimits = archiveLimits{
	maxSourceBytes: maxArchiveSourceBytes,
	maxEntries:     maxArchiveEntries,
}

// Store performs every final filesystem operation through os.Root. Resolving a
// path first preserves in-workspace symlinks; the rooted operation then closes
// the race where a parent is swapped after validation.
type Store struct{}

func NewStore() *Store { return &Store{} }

type secureWorkspace struct {
	root     *os.Root
	realRoot string
}

func (s *Store) DirectoryExists(root, relative string) bool {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return false
	}
	defer workspace.close()
	resolved, err := workspace.resolve(relative)
	if err != nil {
		return false
	}
	info, err := workspace.root.Stat(resolved)
	return err == nil && info.IsDir()
}

func (s *Store) ListDir(root, relative string, maxEntries int) ([]*Node, bool, error) {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return nil, false, err
	}
	defer workspace.close()
	resolved, err := workspace.resolve(relative)
	if err != nil {
		return nil, false, err
	}
	directory, err := workspace.root.Open(resolved)
	if err != nil {
		return nil, false, err
	}
	listing := newBoundedDirectoryListing(maxEntries)
	for {
		entries, readErr := directory.ReadDir(directoryReadBatchSize)
		for _, entry := range entries {
			listing.consider(workspace, relative, entry)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = directory.Close()
			return nil, false, readErr
		}
	}
	if err := directory.Close(); err != nil {
		return nil, false, err
	}
	return listing.result(workspace, relative)
}

func (s *Store) OpenFile(root, relative string) (io.ReadSeekCloser, string, time.Time, error) {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer workspace.close()
	resolved, err := workspace.resolve(relative)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	file, info, err := workspace.openRegular(resolved)
	if err != nil {
		if errors.Is(err, errNotRegularFile) {
			return nil, "", time.Time{}, os.ErrNotExist
		}
		return nil, "", time.Time{}, err
	}
	return file, info.Name(), info.ModTime(), nil
}

func (s *Store) Search(root, query string, limit int) ([]*Node, bool, error) {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return nil, false, err
	}
	defer workspace.close()
	needle := strings.ToLower(query)
	results := make([]*Node, 0, min(limit, 64))
	truncated, visits := false, 0
	walkErr := fs.WalkDir(workspace.root.FS(), ".", func(walkPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if walkPath == "." {
			return nil
		}
		visits++
		if visits > maxSearchVisits {
			truncated = true
			return filepath.SkipAll
		}
		if !strings.Contains(strings.ToLower(entry.Name()), needle) {
			return nil
		}
		node, ok := workspace.nodeFor(path.Dir(walkPath), entry)
		if !ok {
			return nil
		}
		results = append(results, node)
		if len(results) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return nil, false, walkErr
	}
	return results, truncated, nil
}

func (s *Store) WriteArchive(ctx context.Context, root, relative string, destination io.Writer) error {
	return s.writeArchive(ctx, root, relative, destination, defaultArchiveLimits)
}

func (s *Store) writeArchive(
	ctx context.Context,
	root string,
	relative string,
	destination io.Writer,
	limits archiveLimits,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return err
	}
	defer workspace.close()
	base, err := workspace.resolve(relative)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(destination)
	budget := archiveBudget{remainingBytes: limits.maxSourceBytes, remainingEntries: limits.maxEntries}
	walkErr := fs.WalkDir(workspace.root.FS(), base, func(walkPath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if walkPath != base {
			if err := budget.visitEntry(); err != nil {
				return err
			}
		}
		if entry.IsDir() {
			return nil
		}
		return workspace.writeArchiveEntry(ctx, archive, &budget, base, walkPath, entry)
	})
	return errors.Join(walkErr, archive.Close(), ctx.Err())
}

func (w secureWorkspace) writeArchiveEntry(
	ctx context.Context,
	archive *zip.Writer,
	budget *archiveBudget,
	base, walkPath string,
	entry fs.DirEntry,
) error {
	openPath := walkPath
	if entry.Type()&os.ModeSymlink != 0 {
		resolved, err := w.resolve(walkPath)
		if err != nil {
			return nil
		}
		openPath = resolved
	}
	source, info, err := w.openRegular(openPath)
	if err != nil {
		if errors.Is(err, errNotRegularFile) {
			return nil
		}
		return err
	}
	if err := budget.acceptFile(info.Size()); err != nil {
		return errors.Join(err, source.Close())
	}
	relative, err := filepath.Rel(base, walkPath)
	if err != nil {
		return errors.Join(err, source.Close())
	}
	destination, err := archive.Create(filepath.ToSlash(relative))
	if err != nil {
		return errors.Join(err, source.Close())
	}
	return errors.Join(budget.copy(ctx, destination, source), source.Close())
}

type archiveBudget struct {
	remainingBytes   int64
	remainingEntries int
}

func (b *archiveBudget) visitEntry() error {
	if b.remainingEntries <= 0 {
		return ErrArchiveTooLarge
	}
	b.remainingEntries--
	return nil
}

func (b *archiveBudget) acceptFile(size int64) error {
	if size > b.remainingBytes {
		return ErrArchiveTooLarge
	}
	return nil
}

func (b *archiveBudget) copy(ctx context.Context, destination io.Writer, source io.Reader) error {
	written, copyErr := io.Copy(destination, io.LimitReader(contextReader{ctx: ctx, reader: source}, b.remainingBytes+1))
	if written > b.remainingBytes {
		b.remainingBytes = 0
		return errors.Join(copyErr, ErrArchiveTooLarge)
	}
	b.remainingBytes -= written
	return copyErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func (w secureWorkspace) nodeFor(parentRelative string, entry fs.DirEntry) (*Node, bool) {
	name := entry.Name()
	childRelative := path.Join(parentRelative, name)
	isDir := entry.IsDir()
	var size, modTime int64
	if entry.Type()&os.ModeSymlink != 0 {
		resolved, err := w.resolve(childRelative)
		if err != nil {
			return nil, false
		}
		info, err := w.root.Stat(resolved)
		if err != nil {
			return nil, false
		}
		isDir = info.IsDir()
		if !isDir {
			size = info.Size()
		}
		modTime = info.ModTime().UnixMilli()
	} else if info, err := entry.Info(); err == nil {
		if !isDir {
			size = info.Size()
		}
		modTime = info.ModTime().UnixMilli()
	}
	node := &Node{Name: name, Path: childRelative, IsDir: isDir, ModTime: modTime}
	if !isDir {
		node.Size = size
	}
	return node, true
}

type listCandidate struct {
	entry     fs.DirEntry
	node      *Node
	directory bool
	sortName  string
}

func newListCandidate(workspace secureWorkspace, parentRelative string, entry fs.DirEntry) (listCandidate, bool) {
	candidate := listCandidate{entry: entry, directory: entry.IsDir(), sortName: strings.ToLower(entry.Name())}
	if entry.Type()&os.ModeSymlink == 0 {
		return candidate, true
	}
	node, ok := workspace.nodeFor(parentRelative, entry)
	if !ok {
		return listCandidate{}, false
	}
	candidate.node = node
	candidate.directory = node.IsDir
	return candidate, true
}

func listCandidateBefore(left, right listCandidate) bool {
	if left.directory != right.directory {
		return left.directory
	}
	if left.sortName != right.sortName {
		return left.sortName < right.sortName
	}
	return left.entry.Name() < right.entry.Name()
}

type listCandidateHeap []listCandidate

func (h listCandidateHeap) Len() int           { return len(h) }
func (h listCandidateHeap) Less(i, j int) bool { return listCandidateBefore(h[j], h[i]) }
func (h listCandidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *listCandidateHeap) Push(value any)    { *h = append(*h, value.(listCandidate)) }
func (h *listCandidateHeap) Pop() any {
	last := len(*h) - 1
	value := (*h)[last]
	(*h)[last] = listCandidate{}
	*h = (*h)[:last]
	return value
}

type boundedDirectoryListing struct {
	limit      int
	validCount int
	candidates listCandidateHeap
}

func newBoundedDirectoryListing(limit int) *boundedDirectoryListing {
	if limit < 0 {
		limit = 0
	}
	return &boundedDirectoryListing{
		limit:      limit,
		candidates: make(listCandidateHeap, 0, min(limit, directoryReadBatchSize)),
	}
}

func (l *boundedDirectoryListing) consider(workspace secureWorkspace, parentRelative string, entry fs.DirEntry) {
	candidate, ok := newListCandidate(workspace, parentRelative, entry)
	if !ok {
		return
	}
	l.validCount++
	if l.limit == 0 {
		return
	}
	if len(l.candidates) < l.limit {
		heap.Push(&l.candidates, candidate)
		return
	}
	if listCandidateBefore(candidate, l.candidates[0]) {
		l.candidates[0] = candidate
		heap.Fix(&l.candidates, 0)
	}
}

func (l *boundedDirectoryListing) result(workspace secureWorkspace, parentRelative string) ([]*Node, bool, error) {
	sort.Slice(l.candidates, func(i, j int) bool { return listCandidateBefore(l.candidates[i], l.candidates[j]) })
	nodes := make([]*Node, 0, len(l.candidates))
	for _, candidate := range l.candidates {
		node := candidate.node
		if node == nil {
			var ok bool
			node, ok = workspace.nodeFor(parentRelative, candidate.entry)
			if !ok {
				continue
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, l.validCount > l.limit, nil
}

func newSecureWorkspace(root string) (secureWorkspace, error) {
	if !filepath.IsAbs(root) {
		return secureWorkspace{}, ErrInvalidPath
	}
	realRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return secureWorkspace{}, err
	}
	rootHandle, err := os.OpenRoot(realRoot)
	if err != nil {
		return secureWorkspace{}, err
	}
	return secureWorkspace{root: rootHandle, realRoot: realRoot}, nil
}

func (w secureWorkspace) close() { _ = w.root.Close() }

func (w secureWorkspace) openRegular(name string) (*os.File, os.FileInfo, error) {
	info, err := w.root.Stat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errNotRegularFile
	}
	file, err := w.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, errNotRegularFile
	}
	return file, openedInfo, nil
}

func (w secureWorkspace) resolve(relative string) (string, error) {
	if filepath.IsAbs(filepath.FromSlash(relative)) {
		return "", errOutsideWorkspace
	}
	target := filepath.Join(w.realRoot, filepath.FromSlash(relative))
	if !containsPath(target, w.realRoot) {
		return "", errOutsideWorkspace
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	if !containsPath(resolved, w.realRoot) {
		return "", errOutsideWorkspace
	}
	resolvedRelative, err := filepath.Rel(w.realRoot, resolved)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(resolvedRelative), nil
}

func containsPath(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
