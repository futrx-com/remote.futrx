package workspaceide

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/futrx-com/remote.futrx.com/internal/shared/workspacepath"
)

type Service struct {
	authorizer   Authorizer
	baseURL      string
	projectsRoot string
}

func New(baseURL, projectsRoot string, options ...Option) *Service {
	s := &Service{baseURL: baseURL, projectsRoot: projectsRoot}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Service) OpenURL(ctx context.Context, projectID, cwd, rawPath string) (string, error) {
	if err := RequireAccess(ctx, s.authorizer, projectID); err != nil {
		return "", err
	}
	target, err := workspacepath.ResolveFile(rawPath, cwd)
	if err != nil {
		return "", err
	}
	return s.redirectURL(target), nil
}

func (s *Service) redirectURL(target workspacepath.Target) string {
	base := s.baseURL
	folder := target.WorkspaceRoot
	file := target.FilePath
	if slug, containerRoot, ok := workspacepath.ContainerPath(target.WorkspaceRoot, s.projectsRoot); ok {
		base = s.baseURL + slug + "/"
		folder = containerRoot
		if _, containerFile, fileIsInContainer := workspacepath.ContainerPath(target.FilePath, s.projectsRoot); fileIsInContainer {
			file = containerFile
		}
	}
	redirect, err := url.Parse(base)
	if err != nil {
		return base
	}
	query := redirect.Query()
	query.Set("folder", folder)
	if file != "" && file != folder {
		query.Set("payload", openFilePayload(redirect.Host, file, target.Line, target.Column))
	}
	redirect.RawQuery = query.Encode()
	return redirect.String()
}

// openFilePayload builds the VS Code web-workbench payload that makes
// code-server open a file — optionally at an exact line and column. There is
// no first-class file query parameter (coder/code-server#1964), but the
// workbench honors the upstream payload contract, where gotoLineMode makes a
// trailing :line[:column] on the openFile URI position the cursor. The URI
// authority mirrors the page host, matching what the workbench itself uses.
// Verified against code-server 4.121.0.
func openFilePayload(host, file string, line, column int) string {
	fileURI := (&url.URL{Scheme: "vscode-remote", Host: host, Path: file}).String()
	pairs := [][2]string{{"openFile", fileURI}}
	if line > 0 {
		suffix := fmt.Sprintf(":%d", line)
		if column > 0 {
			suffix += fmt.Sprintf(":%d", column)
		}
		pairs[0][1] = fileURI + suffix
		pairs = append(pairs, [2]string{"gotoLineMode", "true"})
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return ""
	}
	return string(encoded)
}
