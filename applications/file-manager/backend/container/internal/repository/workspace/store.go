package workspace

type WorkspaceFileStore struct{}

func New() *WorkspaceFileStore { return &WorkspaceFileStore{} }

func (s *WorkspaceFileStore) DirectoryExists(root, relative string) bool {
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
