package workspacepath

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

type Target struct {
	FilePath      string
	WorkspaceRoot string
	Line          int
	Column        int
}

func Root(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		return ""
	}
	if path == "/workspace" {
		return path
	}
	marker := string(filepath.Separator) + "workspace"
	if strings.HasSuffix(path, marker) {
		return path
	}
	needle := marker + string(filepath.Separator)
	if index := strings.Index(path, needle); index >= 0 {
		return path[:index+len(marker)]
	}
	return path
}

func Contains(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func ResolveFile(rawPath, cwd string) (Target, error) {
	workspaceRoot := Root(cwd)
	if workspaceRoot == "" {
		return Target{}, errors.New("chat workspace is unavailable")
	}
	return resolveFile(rawPath, workspaceRoot)
}

// ResolveFileAtRoot resolves a user-facing path beneath an exact, already
// trusted workspace root. Unlike ResolveFile, it never broadens a nested path
// by searching for a directory named "workspace".
func ResolveFileAtRoot(rawPath, workspaceRoot string) (Target, error) {
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	if !filepath.IsAbs(workspaceRoot) {
		return Target{}, errors.New("chat workspace is unavailable")
	}
	return resolveFile(rawPath, workspaceRoot)
}

func resolveFile(rawPath, workspaceRoot string) (Target, error) {
	rawPath, line, column := parseLineReference(rawPath)
	if rawPath == "" {
		return Target{}, errors.New("path is required")
	}

	cleaned := filepath.Clean(rawPath)
	if IsContainerPath(cleaned) {
		relative := strings.TrimPrefix(cleaned, "/workspace")
		filePath := filepath.Join(workspaceRoot, strings.TrimPrefix(relative, "/"))
		if !Contains(filePath, workspaceRoot) {
			return Target{}, errors.New("path escapes workspace")
		}
		return Target{FilePath: filePath, WorkspaceRoot: workspaceRoot, Line: line, Column: column}, nil
	}

	if !filepath.IsAbs(cleaned) {
		return Target{}, errors.New("path must be absolute")
	}
	if !Contains(cleaned, workspaceRoot) {
		return Target{}, errors.New("path is outside this chat workspace")
	}
	return Target{FilePath: cleaned, WorkspaceRoot: workspaceRoot, Line: line, Column: column}, nil
}

func IsContainerPath(path string) bool {
	return path == "/workspace" || strings.HasPrefix(path, "/workspace/")
}

func ContainerPath(hostPath, projectsRoot string) (slug, containerPath string, ok bool) {
	prefix := filepath.Clean(projectsRoot) + string(filepath.Separator)
	rest, found := strings.CutPrefix(filepath.Clean(hostPath), prefix)
	if !found {
		return "", "", false
	}
	slug, after, found := strings.Cut(rest, string(filepath.Separator))
	if !found || slug == "" {
		return "", "", false
	}
	if after == "workspace" {
		return slug, "/workspace", true
	}
	relative, found := strings.CutPrefix(after, "workspace"+string(filepath.Separator))
	if !found {
		return "", "", false
	}
	return slug, "/workspace/" + filepath.ToSlash(relative), true
}

func parseLineReference(rawPath string) (string, int, int) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", 0, 0
	}

	if hashIndex := strings.Index(rawPath, "#"); hashIndex >= 0 {
		fragment := rawPath[hashIndex+1:]
		rawPath = rawPath[:hashIndex]
		if queryIndex := strings.Index(rawPath, "?"); queryIndex >= 0 {
			rawPath = rawPath[:queryIndex]
		}
		if line, column := parseLineFragment(fragment); line > 0 {
			return strings.TrimSpace(rawPath), line, column
		}
	}
	if queryIndex := strings.Index(rawPath, "?"); queryIndex >= 0 {
		rawPath = rawPath[:queryIndex]
	}

	rawPath, line, column := splitLineSuffix(rawPath)
	return strings.TrimSpace(rawPath), line, column
}

func parseLineFragment(fragment string) (int, int) {
	fragment = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(fragment), "l"))
	if fragment == "" {
		return 0, 0
	}
	parts := strings.SplitN(fragment, ":", 2)
	line, err := strconv.Atoi(parts[0])
	if err != nil || line <= 0 {
		return 0, 0
	}
	if len(parts) == 2 {
		column, err := strconv.Atoi(parts[1])
		if err == nil && column > 0 {
			return line, column
		}
	}
	return line, 0
}

func splitLineSuffix(rawPath string) (string, int, int) {
	rawPath = strings.TrimSpace(rawPath)
	lastColon := strings.LastIndex(rawPath, ":")
	if lastColon < 0 {
		return rawPath, 0, 0
	}
	lastNumber, err := strconv.Atoi(rawPath[lastColon+1:])
	if err != nil || lastNumber <= 0 {
		return rawPath, 0, 0
	}

	beforeLast := rawPath[:lastColon]
	secondColon := strings.LastIndex(beforeLast, ":")
	if secondColon >= 0 {
		line, err := strconv.Atoi(beforeLast[secondColon+1:])
		if err == nil && line > 0 {
			return beforeLast[:secondColon], line, lastNumber
		}
	}
	return beforeLast, lastNumber, 0
}
