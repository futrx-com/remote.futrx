package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// expirer is implemented by any payload type carried through signedPayload,
// so verify can enforce expiry generically without depending on any one
// payload shape (Session, a pending-login challenge, a pending-enrollment
// token, ...). Each payload type owns its own expiry field/computation.
type expirer interface {
	expired(now time.Time) bool
}

// signedPayload is a small generic HMAC sign/verify/expire primitive,
// extracted from what was sessionCodec so the same envelope format
// (b64(json) + "." + b64(hmac)) can back real sessions, pending 2FA-login
// challenges, and pending 2FA-enrollment tokens without three copies of this
// logic.
//
// Every payload type is signed with the same account-wide session key, and
// their JSON shapes overlap (Session, pendingLogin and pendingEnrollment all
// carry "email"/"exp"). Without separation a token minted for one purpose
// verifies as another - in particular a pending-2FA-login token would decode
// into a fully valid Session, letting a caller who knows only the first
// factor skip the second. domain fixes the purpose into the MAC so a token
// only ever verifies against the codec that produced it.
//
// The session codec deliberately uses the empty domain, which reproduces the
// original MAC byte-for-byte, so sessions issued before this change keep
// verifying. Every non-session payload must use a non-empty domain.
type signedPayload[T expirer] struct {
	key    []byte
	domain string
}

// newSessionPayload is the sole empty-domain construction path. The empty
// domain intentionally preserves session cookies issued before purpose
// binding was introduced.
func newSessionPayload(key []byte) signedPayload[Session] {
	return signedPayload[Session]{key: key}
}

func newPendingLoginPayload(key []byte) signedPayload[pendingLogin] {
	return signedPayload[pendingLogin]{key: key, domain: "pending-login/v1"}
}

func newPendingEnrollmentPayload(key []byte) signedPayload[pendingEnrollment] {
	return signedPayload[pendingEnrollment]{key: key, domain: "2fa-enrollment/v1"}
}

// mac binds the codec's domain to the payload so the same key cannot produce
// an interchangeable token for two different purposes. The 0x00 separator
// keeps the domain unambiguously delimited from the payload.
func (p signedPayload[T]) mac(b64 string) string {
	mac := hmac.New(sha256.New, p.key)
	if p.domain != "" {
		mac.Write([]byte(p.domain))
		mac.Write([]byte{0})
	}
	mac.Write([]byte(b64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (p signedPayload[T]) sign(v T) string {
	body, _ := json.Marshal(v)
	b64 := base64.RawURLEncoding.EncodeToString(body)
	return b64 + "." + p.mac(b64)
}

func (p signedPayload[T]) verify(raw string) (T, error) {
	var zero T
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 {
		return zero, errors.New("malformed")
	}
	want := p.mac(parts[0])
	if !hmac.Equal([]byte(parts[1]), []byte(want)) {
		return zero, errors.New("bad signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return zero, err
	}
	if v.expired(time.Now()) {
		return zero, errors.New("expired")
	}
	return v, nil
}
