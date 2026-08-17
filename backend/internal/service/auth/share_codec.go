package auth

import (
	"encoding/json"
	"errors"
	"time"
)

// ShareCookieName carries the signed grant a public-preview visitor receives
// after their share token was accepted once. It is set without a Domain, so
// the browser scopes it to the single preview hostname it was issued for and
// it never reaches the main application or another project.
const ShareCookieName = "remote_share"

// sharePassPurpose keeps share passes from verifying as platform sessions even
// though both are signed with DATA_DIR/session.key.
const sharePassPurpose = "share-pass.v1"

// SharePass is the payload inside ShareCookieName: which preview host and port
// the visitor may see, which share link granted it, and when that stops.
type SharePass struct {
	Slug    string `json:"slug"`
	Port    int    `json:"port"`
	ShareID string `json:"sid"`
	Exp     int64  `json:"exp"`
}

type sharePassCodec struct {
	key []byte
}

func newSharePassCodec(key []byte) *sharePassCodec {
	return &sharePassCodec{key: key}
}

func (c *sharePassCodec) sign(pass SharePass) string {
	body, _ := json.Marshal(pass)
	return signPayload(c.key, sharePassPurpose, body)
}

func (c *sharePassCodec) verify(value string) (*SharePass, error) {
	if value == "" {
		return nil, errors.New("missing share cookie")
	}
	body, err := openPayload(c.key, sharePassPurpose, value)
	if err != nil {
		return nil, err
	}
	var pass SharePass
	if err := json.Unmarshal(body, &pass); err != nil {
		return nil, err
	}
	if pass.Slug == "" || pass.Port == 0 || pass.ShareID == "" {
		return nil, errors.New("incomplete share pass")
	}
	if time.Now().Unix() > pass.Exp {
		return nil, errors.New("expired")
	}
	return &pass, nil
}
