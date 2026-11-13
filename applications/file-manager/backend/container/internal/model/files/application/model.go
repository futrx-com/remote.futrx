package application

type Node struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime int64
}

// Listing is the contents of a single directory within the workspace.
type Listing struct {
	Path      string
	Entries   []*Node
	Truncated bool
}

// SearchResult is a flat set of workspace entries whose name matched a query.
type SearchResult struct {
	Entries   []*Node
	Truncated bool
}

type Archive struct {
	Name     string
	Root     string
	Relative string
}

// PublishedArchive is a finished ZIP waiting in the workspace for the browser.
// Path is relative to the workspace.
type PublishedArchive struct {
	Path string
}
