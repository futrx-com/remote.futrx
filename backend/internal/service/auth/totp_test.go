package auth

import (
	"strings"
	"testing"
	"time"
)

// TestTOTPCodeMatchesRFC6238Vectors checks TOTPCode against the published
// RFC 6238 Appendix B SHA1 test vectors (which use 8-digit output), reduced
// to our 6-digit output via mod 1e6 - valid because 1e6 divides 1e8, so
// (x mod 1e8) mod 1e6 == x mod 1e6.
func TestTOTPCodeMatchesRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890") // RFC 6238 Appendix B SHA1 secret

	cases := []struct {
		unixSeconds int64
		want8Digit  string
		want6Digit  string
	}{
		{59, "94287082", "287082"},
		{1111111109, "07081804", "081804"},
		{1111111111, "14050471", "050471"},
		{1234567890, "89005924", "005924"},
		{2000000000, "69279037", "279037"},
	}

	for _, c := range cases {
		got := TOTPCode(secret, time.Unix(c.unixSeconds, 0).UTC())
		if got != c.want6Digit {
			t.Errorf("TOTPCode(t=%d) = %q, want %q (derived from RFC 6238 8-digit vector %q)",
				c.unixSeconds, got, c.want6Digit, c.want8Digit)
		}
	}
}

func TestVerifyTOTPCodeAllowsOneStepSkew(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)
	code := TOTPCode(secret, now)

	if !verifyTOTPCode(secret, code, now) {
		t.Fatal("code did not verify at the same instant it was generated")
	}
	if !verifyTOTPCode(secret, code, now.Add(30*time.Second)) {
		t.Fatal("code did not verify one step later (should tolerate +1 skew)")
	}
	if !verifyTOTPCode(secret, code, now.Add(-30*time.Second)) {
		t.Fatal("code did not verify one step earlier (should tolerate -1 skew)")
	}
	if verifyTOTPCode(secret, code, now.Add(90*time.Second)) {
		t.Fatal("code verified three steps later, want rejection outside +/-1 skew")
	}
	if verifyTOTPCode(secret, "000000", now) {
		t.Fatal("wrong code verified")
	}
}

func TestGenerateTOTPSecretIsRandomAndCorrectLength(t *testing.T) {
	a, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	b, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if len(a) != totpSecretBytes || len(b) != totpSecretBytes {
		t.Fatalf("secret lengths = %d, %d, want %d", len(a), len(b), totpSecretBytes)
	}
	if string(a) == string(b) {
		t.Fatal("two generated secrets were identical")
	}
}

func TestTOTPProvisioningURIContainsExpectedFields(t *testing.T) {
	secret := []byte("12345678901234567890")
	uri := totpProvisioningURI("remote.futrx", "user@example.com", secret)

	wantSubstrings := []string{
		"otpauth://totp/",
		"remote.futrx",
		"user@example.com",
		"secret=",
		"issuer=remote.futrx",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(uri, want) {
			t.Errorf("provisioning URI %q missing %q", uri, want)
		}
	}
}
