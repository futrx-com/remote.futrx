package auth

import (
	"errors"
	"time"
)

// ShareCookieName carries the signed grant a public-preview visitor receives
// after their share token was accepted once. It is set without a Domain, so
// the browser scopes it to the single preview hostname it was issued for and
// it never reaches the main application or another project.
const ShareCookieName = "remote_share"

// sharePassDomain keeps share passes from verifying as platform sessions even
// though both are signed with DATA_DIR/session.key.
//
// This is the same separation signedPayload gives every other non-session
// payload, and it matters more here than anywhere: a share pass is handed to
// an anonymous visitor. Without a domain, a JSON body carrying "exp" would
// verify against the session codec, and the person holding a preview link
// would hold a session.
//
// The value is the one this fork already signs with. Upstream's other domains
// read "<name>/v1"; this one keeps its dot because changing it would change
// the MAC and quietly invalidate every share cookie a visitor is holding.
const sharePassDomain = "share-pass.v1"

// SharePass is the payload inside ShareCookieName: which preview host and port
// the visitor may see, which share link granted it, and when that stops.
type SharePass struct {
	Slug    string `json:"slug"`
	Port    int    `json:"port"`
	ShareID string `json:"sid"`
	Exp     int64  `json:"exp"`
}

func (s SharePass) expired(now time.Time) bool { return now.Unix() > s.Exp }

type sharePassCodec struct {
	payload signedPayload[SharePass]
}

func newSharePassCodec(key []byte) *sharePassCodec {
	return &sharePassCodec{payload: signedPayload[SharePass]{key: key, domain: sharePassDomain}}
}

func (c *sharePassCodec) sign(pass SharePass) string {
	return c.payload.sign(pass)
}

// verify authenticates a cookie value. Expiry is enforced by signedPayload;
// the completeness check below is this payload's own, because a pass missing
// its slug or port would authorise nothing and is a bug rather than a denial.
func (c *sharePassCodec) verify(value string) (*SharePass, error) {
	if value == "" {
		return nil, errors.New("missing share cookie")
	}
	pass, err := c.payload.verify(value)
	if err != nil {
		return nil, err
	}
	if pass.Slug == "" || pass.Port == 0 || pass.ShareID == "" {
		return nil, errors.New("incomplete share pass")
	}
	return &pass, nil
}
