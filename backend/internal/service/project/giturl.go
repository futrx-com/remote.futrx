package project

import "net/url"

// ValidGitURL reports whether s is an acceptable repository URL to clone
// from on project creation. An empty string is valid — the field is
// optional. Only plain public https:// URLs are accepted for now: no
// ssh://, no git@host: shorthand, no file:// or other local paths.
func ValidGitURL(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Host != ""
}
