// Package share owns public preview links: time-boxed, revocable grants that
// let someone without a platform account open one project's
// <slug>--<port>.dev.<host> preview. A share never widens access to the IDE,
// the agent browser, or the main application.
package share

import (
	"errors"
	"time"
)

// ID identifies one share link inside a project.
type ID string

const (
	// DefaultTTL applies when the caller does not ask for a lifetime.
	DefaultTTL = 24 * time.Hour
	// MinTTL keeps a link from being created already useless.
	MinTTL = time.Hour
	// MaxTTL bounds how long an unauthenticated grant may survive.
	MaxTTL = 30 * 24 * time.Hour

	// TokenBytes is the entropy behind a token before base64 encoding.
	TokenBytes = 32
	// MaxLabelLength bounds the operator's reminder text.
	MaxLabelLength = 80
	// MaxPerProject bounds how many live links one project can hold.
	MaxPerProject = 50

	// MinPort and MaxPort mirror the preview host regex in Caddy and in
	// HandleTLSAsk.
	MinPort = 1024
	MaxPort = 65535

	// AgentBrowserPort is the in-container noVNC port. It is never shareable:
	// that browser is a signed-in session the project's members and agents
	// drive together.
	AgentBrowserPort = 6080

	// IDEProxyPort is the socket-activated code-server proxy and CDPPort the
	// agent browser's DevTools endpoint. Both are platform plumbing, never a
	// user application, and a public link to either would bypass edge auth.
	IDEProxyPort  = 8842
	IDEDirectPort = 8081
	CDPPort       = 9222
)

// reservedPorts are in-container platform listeners that must never be
// exposed through a public link.
var reservedPorts = map[int]struct{}{
	AgentBrowserPort: {},
	IDEProxyPort:     {},
	IDEDirectPort:    {},
	CDPPort:          {},
}

var (
	ErrInvalidPort      = errors.New("preview port must be between 1024 and 65535")
	ErrPortNotShareable = errors.New("this port is platform plumbing (agent browser, IDE, or DevTools) and cannot be shared publicly")
	ErrInvalidTTL       = errors.New("share lifetime must be between 1 hour and 30 days")
	ErrTooManyShares    = errors.New("this project already has the maximum number of active share links")
	ErrNotFound         = errors.New("share link not found")
	ErrUnavailable      = errors.New("share link store is not configured")
)

// Share is the stored record of one public preview link.
//
// The token itself is never persisted — only its SHA-256 digest — so a copy of
// DATA_DIR cannot be replayed against a preview host.
type Share struct {
	ID        ID     `json:"id"`
	TokenHash string `json:"tokenHash"`
	Port      int    `json:"port"`
	Label     string `json:"label,omitempty"`
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	RevokedAt int64  `json:"revokedAt,omitempty"`
}

// Active reports whether the share can still authorize a request at nowMilli.
func (s Share) Active(nowMilli int64) bool {
	return s.RevokedAt == 0 && s.ExpiresAt > nowMilli
}

// CreateInput is the caller-supplied half of a new share link.
type CreateInput struct {
	Port     int    `json:"port"`
	TTLHours int    `json:"ttlHours,omitempty"`
	Label    string `json:"label,omitempty"`
}

// Created carries the one and only copy of the plaintext token alongside the
// persisted record and the project slug the link belongs to.
type Created struct {
	Share Share
	Token string
	Slug  string
}

// ShareablePort reports whether port may back a public preview link.
func ShareablePort(port int) error {
	if port < MinPort || port > MaxPort {
		return ErrInvalidPort
	}
	if _, reserved := reservedPorts[port]; reserved {
		return ErrPortNotShareable
	}
	return nil
}
