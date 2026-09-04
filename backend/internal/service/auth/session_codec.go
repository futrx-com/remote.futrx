package auth

import "time"

const sessionDuration = 30 * 24 * time.Hour

func (s Session) expired(now time.Time) bool {
	return now.Unix() > s.Exp
}

// sessionCodec owns the signed session representation. Its methods stay
// package-private so Service remains the supported authentication facade.
type sessionCodec struct {
	payload signedPayload[Session]
}

func newSessionCodec(key []byte) *sessionCodec {
	// The empty domain is intentional: it reproduces the pre-domain MAC, so
	// sessions issued before domain separation existed keep verifying. Every
	// other payload type must use a distinct, non-empty domain.
	return &sessionCodec{payload: newSessionPayload(key)}
}

func (c *sessionCodec) sign(user User, sid string) string {
	now := time.Now()
	session := Session{
		Email: user.Email,
		Sub:   user.Sub,
		Iat:   now.Unix(),
		Exp:   now.Add(sessionDuration).Unix(),
		SID:   sid,
	}
	return c.payload.sign(session)
}

func (c *sessionCodec) verify(value string) (*Session, error) {
	session, err := c.payload.verify(value)
	if err != nil {
		return nil, err
	}
	return &session, nil
}
