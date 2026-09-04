package githistory

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/shared/workspacepath"
)

const (
	maxRepositoryDepth = 6
	maxDiffLength      = 768 * 1024
	defaultCommitLimit = 80
	maxCommitLimit     = 200
)

var skippedDirectories = []string{
	".git", ".agents", ".browser", ".cache", ".claude", ".codex", ".minimax", ".media", ".vscode",
	"node_modules", ".next", "dist", "build", "out", "coverage", "vendor", "tmp", "__pycache__",
}

type Client interface {
	DirectoryExists(path string) bool
	IsRepository(path string) bool
	DiscoverRepositories(root string, maxDepth int, skippedDirectories []string) []string
	Head(ctx context.Context, repositoryPath string) (string, error)
	CurrentRef(ctx context.Context, repositoryPath string) (string, error)
	Status(ctx context.Context, repositoryPath string) (string, error)
	Log(ctx context.Context, repositoryPath string, limit int) (string, error)
	CommitDetails(ctx context.Context, repositoryPath, sha string) (string, error)
	CommitDiff(ctx context.Context, repositoryPath, sha string) (string, error)
	ResolveCommit(ctx context.Context, repositoryPath, sha string) (string, error)
	StageAll(ctx context.Context, repositoryPath string) error
	CreateCheckpoint(ctx context.Context, repositoryPath, message string) error
	CheckoutDetached(ctx context.Context, repositoryPath, sha string) (string, error)
}

type DirtyWorkingTreeError struct {
	Files []string
}

func (e *DirtyWorkingTreeError) Error() string {
	return "dirty working tree"
}

type ConflictError struct {
	Err error
}

func (e *ConflictError) Error() string {
	return e.Err.Error()
}

func (e *ConflictError) Unwrap() error {
	return e.Err
}

type Service struct {
	client Client
}

func New(client Client) *Service {
	return &Service{client: client}
}

func (s *Service) Repositories(ctx context.Context, cwd string) (Repositories, error) {
	workspaceRoot, err := s.workspaceRoot(cwd)
	if err != nil {
		return Repositories{}, err
	}
	repositories := []Repository{}
	for _, path := range s.client.DiscoverRepositories(workspaceRoot, maxRepositoryDepth, skippedDirectories) {
		repository, err := s.describeRepository(ctx, workspaceRoot, path)
		if err == nil {
			repositories = append(repositories, repository)
		}
	}
	sort.Slice(repositories, func(i, j int) bool {
		if repositories[i].ID == "." {
			return true
		}
		if repositories[j].ID == "." {
			return false
		}
		return repositories[i].RelativePath < repositories[j].RelativePath
	})
	return Repositories{WorkspaceRoot: workspaceRoot, Repos: repositories}, nil
}

func (s *Service) Commits(ctx context.Context, cwd, rawRepository string, limit int) (Commits, error) {
	repositoryPath, repository, err := s.repository(ctx, cwd, rawRepository)
	if err != nil {
		return Commits{}, err
	}
	if limit < 1 {
		limit = defaultCommitLimit
	}
	if limit > maxCommitLimit {
		limit = maxCommitLimit
	}
	commits, err := s.listCommits(ctx, repositoryPath, repository.CurrentSHA, limit)
	if err != nil {
		return Commits{}, err
	}
	return Commits{Repo: repository, Commits: commits}, nil
}

func (s *Service) Diff(ctx context.Context, cwd, rawRepository, rawSHA string) (Diff, error) {
	repositoryPath, repository, err := s.repository(ctx, cwd, rawRepository)
	if err != nil {
		return Diff{}, err
	}
	commit, err := s.commit(ctx, repositoryPath, repository.CurrentSHA, rawSHA)
	if err != nil {
		return Diff{}, err
	}
	diff, truncated, err := s.diff(ctx, repositoryPath, commit.SHA)
	if err != nil {
		return Diff{}, err
	}
	return Diff{Repo: repository, Commit: commit, Diff: diff, Truncated: truncated}, nil
}

func (s *Service) Checkout(ctx context.Context, cwd string, input CheckoutInput) (Checkout, error) {
	repositoryPath, _, err := s.repository(ctx, cwd, input.Repo)
	if err != nil {
		return Checkout{}, err
	}
	sha, err := s.verifyCommit(ctx, repositoryPath, input.SHA)
	if err != nil {
		return Checkout{}, err
	}

	_, dirtyFiles, err := s.repositoryStatus(ctx, repositoryPath)
	if err != nil {
		return Checkout{}, err
	}
	var checkpointSHA string
	if len(dirtyFiles) > 0 {
		if strings.TrimSpace(input.CheckpointMessage) == "" {
			return Checkout{}, &DirtyWorkingTreeError{Files: dirtyFiles}
		}
		checkpointSHA, err = s.checkpointChanges(ctx, repositoryPath, input.CheckpointMessage)
		if err != nil {
			return Checkout{}, &ConflictError{Err: err}
		}
	}

	output, err := s.client.CheckoutDetached(ctx, repositoryPath, sha)
	if err != nil {
		return Checkout{}, &ConflictError{Err: err}
	}
	workspaceRoot, err := s.workspaceRoot(cwd)
	if err != nil {
		return Checkout{}, err
	}
	repository, err := s.describeRepository(ctx, workspaceRoot, repositoryPath)
	if err != nil {
		return Checkout{}, err
	}
	return Checkout{Repo: repository, Output: output, CheckpointSHA: checkpointSHA}, nil
}

func (s *Service) repository(ctx context.Context, cwd, rawRepository string) (string, Repository, error) {
	workspaceRoot, err := s.workspaceRoot(cwd)
	if err != nil {
		return "", Repository{}, err
	}
	repositoryPath, err := s.resolveRepositoryPath(rawRepository, workspaceRoot)
	if err != nil {
		return "", Repository{}, err
	}
	repository, err := s.describeRepository(ctx, workspaceRoot, repositoryPath)
	if err != nil {
		return "", Repository{}, err
	}
	return repositoryPath, repository, nil

}

func (s *Service) workspaceRoot(cwd string) (string, error) {
	workspaceRoot := workspacepath.Root(cwd)
	if workspaceRoot == "" {
		return "", errors.New("chat workspace is unavailable")
	}
	if !s.client.DirectoryExists(workspaceRoot) {
		return "", errors.New("chat workspace is unavailable on this host")
	}
	return workspaceRoot, nil
}

func (s *Service) resolveRepositoryPath(rawRepository, workspaceRoot string) (string, error) {
	workspaceRoot = filepath.Clean(workspaceRoot)
	rawRepository = strings.TrimSpace(rawRepository)
	if rawRepository == "" || rawRepository == "." {
		if !s.client.IsRepository(workspaceRoot) {
			return "", errors.New("workspace root is not a git repository")
		}
		return workspaceRoot, nil
	}

	cleaned := filepath.Clean(rawRepository)
	var repositoryPath string
	if isContainerWorkspacePath(cleaned) {
		relative := strings.TrimPrefix(cleaned, "/workspace")
		repositoryPath = filepath.Join(workspaceRoot, strings.TrimPrefix(relative, "/"))
	} else if filepath.IsAbs(cleaned) {
		repositoryPath = cleaned
	} else {
		repositoryPath = filepath.Join(workspaceRoot, filepath.FromSlash(rawRepository))
	}
	repositoryPath = filepath.Clean(repositoryPath)
	if !workspacepath.Contains(repositoryPath, workspaceRoot) {
		return "", errors.New("repository is outside this chat workspace")
	}
	if !s.client.IsRepository(repositoryPath) {
		return "", errors.New("not a git repository")
	}
	return repositoryPath, nil
}

func (s *Service) describeRepository(ctx context.Context, workspaceRoot, repositoryPath string) (Repository, error) {
	if !s.client.IsRepository(repositoryPath) {
		return Repository{}, errors.New("not a git repository")
	}
	sha, err := s.client.Head(ctx, repositoryPath)
	if err != nil {
		return Repository{}, err
	}
	ref, err := s.client.CurrentRef(ctx, repositoryPath)
	if err != nil || strings.TrimSpace(ref) == "" {
		ref = "detached"
	}
	_, dirtyFiles, _ := s.repositoryStatus(ctx, repositoryPath)
	id, relativePath := repositoryID(workspaceRoot, repositoryPath)
	name := filepath.Base(repositoryPath)
	if id == "." {
		name = filepath.Base(workspaceRoot)
	}
	return Repository{
		ID:           id,
		Name:         name,
		Path:         repositoryPath,
		RelativePath: relativePath,
		CurrentRef:   ref,
		CurrentSHA:   strings.TrimSpace(sha),
		Dirty:        len(dirtyFiles) > 0,
		DirtyFiles:   dirtyFiles,
	}, nil
}

func (s *Service) listCommits(ctx context.Context, repositoryPath, currentSHA string, limit int) ([]Commit, error) {
	output, err := s.client.Log(ctx, repositoryPath, limit)
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return []Commit{}, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	commits := make([]Commit, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		commit, err := parseCommitLine(line, currentSHA)
		if err == nil {
			commits = append(commits, commit)
		}
	}
	return commits, nil
}

func (s *Service) commit(ctx context.Context, repositoryPath, currentSHA, rawSHA string) (Commit, error) {
	sha, err := s.verifyCommit(ctx, repositoryPath, rawSHA)
	if err != nil {
		return Commit{}, err
	}
	output, err := s.client.CommitDetails(ctx, repositoryPath, sha)
	if err != nil {
		return Commit{}, err
	}
	return parseCommitLine(output, currentSHA)
}

func (s *Service) diff(ctx context.Context, repositoryPath, sha string) (string, bool, error) {
	output, err := s.client.CommitDiff(ctx, repositoryPath, sha)
	if err != nil {
		return "", false, err
	}
	truncated := false
	if len(output) > maxDiffLength {
		output = output[:maxDiffLength] + "\n\n[diff truncated]"
		truncated = true
	}
	return output, truncated, nil
}

func (s *Service) repositoryStatus(ctx context.Context, repositoryPath string) (string, []string, error) {
	status, err := s.client.Status(ctx, repositoryPath)
	if err != nil {
		return "", nil, err
	}
	return status, parseDirtyFiles(status), nil
}

func (s *Service) checkpointChanges(ctx context.Context, repositoryPath, message string) (string, error) {
	message = sanitizeCheckpointMessage(message)
	if message == "" {
		return "", errors.New("checkpoint message is required")
	}
	if err := s.client.StageAll(ctx, repositoryPath); err != nil {
		return "", err
	}
	if _, dirtyFiles, err := s.repositoryStatus(ctx, repositoryPath); err != nil {
		return "", err
	} else if len(dirtyFiles) == 0 {
		return "", errors.New("there are no changes to checkpoint")
	}
	if err := s.client.CreateCheckpoint(ctx, repositoryPath, message); err != nil {
		return "", err
	}
	sha, err := s.client.Head(ctx, repositoryPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func (s *Service) verifyCommit(ctx context.Context, repositoryPath, rawSHA string) (string, error) {
	sha := strings.TrimSpace(rawSHA)
	if !validCommitID(sha) {
		return "", errors.New("invalid commit")
	}
	canonical, err := s.client.ResolveCommit(ctx, repositoryPath, sha)
	if err != nil {
		return "", errors.New("commit not found")
	}
	return strings.TrimSpace(canonical), nil
}

func parseCommitLine(line, currentSHA string) (Commit, error) {
	parts := strings.SplitN(line, "\x1f", 6)
	if len(parts) != 6 {
		return Commit{}, errors.New("invalid commit data")
	}
	authorDate, _ := strconv.ParseInt(parts[4], 10, 64)
	sha := strings.TrimSpace(parts[0])
	return Commit{
		SHA:         sha,
		ShortSHA:    strings.TrimSpace(parts[1]),
		Subject:     strings.TrimSpace(parts[5]),
		AuthorName:  strings.TrimSpace(parts[2]),
		AuthorEmail: strings.TrimSpace(parts[3]),
		AuthorDate:  authorDate,
		IsHead:      currentSHA != "" && sha == currentSHA,
	}, nil
}

func parseDirtyFiles(status string) []string {
	lines := strings.Split(strings.TrimSpace(status), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func sanitizeCheckpointMessage(message string) string {
	message = strings.TrimSpace(message)
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 200 {
		message = strings.TrimSpace(message[:200])
	}
	return message
}

func repositoryID(workspaceRoot, repositoryPath string) (string, string) {
	relative, err := filepath.Rel(workspaceRoot, repositoryPath)
	if err != nil || relative == "." {
		return ".", "."
	}
	relative = filepath.ToSlash(relative)
	return relative, relative
}

func isContainerWorkspacePath(path string) bool {
	return path == "/workspace" || strings.HasPrefix(path, "/workspace/")
}

func validCommitID(sha string) bool {
	if len(sha) < 4 || len(sha) > 64 {
		return false
	}
	for _, character := range sha {
		if (character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f') ||
			(character >= 'A' && character <= 'F') {
			continue
		}
		return false
	}
	return true
}
