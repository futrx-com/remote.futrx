package workspace

import (
	"container/heap"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

func (s *WorkspaceFileStore) ListDir(root, relative string, maxEntries int) ([]*model.Node, bool, error) {
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
		entries, readErr := directory.ReadDir(config.DirectoryReadBatchSize)
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

// nodeFor builds a listing node for a directory entry. Symlinks are resolved and
// dropped if they escape the workspace; the returned Path is always the entry's
// workspace-relative path so the client can request it back safely.
func (w secureWorkspace) nodeFor(parentRelative string, entry fs.DirEntry) (*model.Node, bool) {
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

	node := &model.Node{Name: name, Path: childRelative, IsDir: isDir, ModTime: modTime}
	if !isDir {
		node.Size = size
	}
	return node, true
}

type listCandidate struct {
	entry     fs.DirEntry
	node      *model.Node
	directory bool
	sortName  string
}

func newListCandidate(
	workspace secureWorkspace,
	parentRelative string,
	entry fs.DirEntry,
) (listCandidate, bool) {
	candidate := listCandidate{
		entry:     entry,
		directory: entry.IsDir(),
		sortName:  strings.ToLower(entry.Name()),
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return candidate, true
	}
	node, ok := workspace.nodeFor(parentRelative, entry)
	if !ok {
		return listCandidate{}, false
	}
	candidate.node = node
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

// listCandidateHeap keeps the worst retained entry at index zero, allowing a
// complete directory scan to preserve global sort order with O(limit) memory.
type listCandidateHeap []listCandidate

func (h listCandidateHeap) Len() int { return len(h) }

func (h listCandidateHeap) Less(i, j int) bool {
	return listCandidateBefore(h[j], h[i])
}

func (h listCandidateHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *listCandidateHeap) Push(value any) {
	*h = append(*h, value.(listCandidate))
}

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
	capacity := min(limit, config.DirectoryReadBatchSize)
	return &boundedDirectoryListing{
		limit:      limit,
		candidates: make(listCandidateHeap, 0, capacity),
	}
}

func (l *boundedDirectoryListing) consider(
	workspace secureWorkspace,
	parentRelative string,
	entry fs.DirEntry,
) {
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

func (l *boundedDirectoryListing) result(
	workspace secureWorkspace,
	parentRelative string,
) ([]*model.Node, bool, error) {
	sort.Slice(l.candidates, func(i, j int) bool {
		return listCandidateBefore(l.candidates[i], l.candidates[j])
	})
	nodes := make([]*model.Node, 0, len(l.candidates))
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
