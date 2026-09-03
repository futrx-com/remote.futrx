package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 mandates SHA-1 for TOTP interop with authenticator apps.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const (
	totpSecretBytes = 20
	totpDigits      = 6
	totpStep        = 30 * time.Second
	totpSkewSteps   = 1
)

// GenerateTOTPSecret returns a fresh, random 20-byte (160-bit) TOTP secret,
// the size recommended by RFC 4226 §4/RFC 6238 for HMAC-SHA1.
func GenerateTOTPSecret() ([]byte, error) {
	secret := make([]byte, totpSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate TOTP secret: %w", err)
	}
	return secret, nil
}

// TOTPCode computes the RFC 6238 time-based one-time code for secret at t,
// using the standard 30-second step and 6-digit output.
func TOTPCode(secret []byte, t time.Time) string {
	counter := uint64(t.Unix()) / uint64(totpStep.Seconds())
	return hotpCode(secret, counter)
}

func hotpCode(secret []byte, counter uint64) string {
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)

	mac := hmac.New(sha1.New, secret)
	mac.Write(counterBytes[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	code := truncated % pow10(totpDigits)
	return fmt.Sprintf("%0*d", totpDigits, code)
}

func pow10(n int) uint32 {
	result := uint32(1)
	for range n {
		result *= 10
	}
	return result
}

// verifyTOTPCode checks code against secret, allowing +/- one 30-second step
// of clock skew (the ±1 step window recommended by RFC 6238 §5.2), using a
// constant-time comparison to avoid leaking timing information.
func verifyTOTPCode(secret []byte, code string, at time.Time) bool {
	_, ok := verifyTOTPCounter(secret, code, at)
	return ok
}

// verifyTOTPCounter is verifyTOTPCode plus the time-step counter the code
// matched on. Callers that must reject replays (RFC 6238 SS5.2: a code should
// only ever be accepted once) record that counter and refuse anything at or
// below it next time.
func verifyTOTPCounter(secret []byte, code string, at time.Time) (uint64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	counter := uint64(at.Unix()) / uint64(totpStep.Seconds())
	for skew := -totpSkewSteps; skew <= totpSkewSteps; skew++ {
		c := counter
		if skew < 0 {
			if c < uint64(-skew) {
				continue
			}
			c -= uint64(-skew)
		} else {
			c += uint64(skew)
		}
		want := hotpCode(secret, c)
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return c, true
		}
	}
	return 0, false
}

// totpProvisioningURI builds the otpauth://totp/... URI that authenticator
// apps consume for QR/manual enrollment.
func totpProvisioningURI(issuer, accountEmail string, secret []byte) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(accountEmail)
	query := url.Values{}
	query.Set("secret", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret))
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(totpDigits))
	query.Set("period", strconv.Itoa(int(totpStep.Seconds())))
	return "otpauth://totp/" + label + "?" + query.Encode()
}
