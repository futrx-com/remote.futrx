package httphandlers

import (
	"encoding/base32"
	"net/http"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

func TestSecuritySummaryRequiresSession(t *testing.T) {
	mux, _ := newTwoFactorTestMux(t)
	rec := newCookieJar(mux).do(t, http.MethodGet, "/api/me/security", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("summary status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := decodeJSON[map[string]string](t, rec)["error"]; got != "authentication required" {
		t.Fatalf("error = %q, want authentication required", got)
	}
}

func TestSecurityDisableTwoFactorProof(t *testing.T) {
	for _, tc := range []struct {
		name       string
		validProof bool
		wantStatus int
	}{
		{name: "recovery code", validProof: true, wantStatus: http.StatusNoContent},
		{name: "wrong code", wantStatus: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, auth := newTwoFactorTestMux(t)
			jar := newCookieJar(mux)
			if rec := jar.do(t, http.MethodPost, "/auth/local/claim", map[string]string{
				"email": "admin@example.com", "password": "correct horse battery staple", "setupToken": setupTokenFor(t, auth),
			}); rec.Code != http.StatusCreated {
				t.Fatalf("claim status = %d, body = %s", rec.Code, rec.Body.String())
			}
			enrollRec := jar.do(t, http.MethodPost, "/api/me/security/2fa/enroll", nil)
			if enrollRec.Code != http.StatusOK {
				t.Fatalf("enroll status = %d, body = %s", enrollRec.Code, enrollRec.Body.String())
			}
			enroll := decodeJSON[struct {
				EnrollmentToken string `json:"enrollmentToken"`
				Secret          string `json:"secret"`
			}](t, enrollRec)
			secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enroll.Secret)
			if err != nil {
				t.Fatalf("decode secret: %v", err)
			}
			confirmRec := jar.do(t, http.MethodPost, "/api/me/security/2fa/confirm", map[string]string{
				"enrollmentToken": enroll.EnrollmentToken, "code": serviceauth.TOTPCode(secret, time.Now()),
			})
			if confirmRec.Code != http.StatusOK {
				t.Fatalf("confirm status = %d, body = %s", confirmRec.Code, confirmRec.Body.String())
			}
			confirmed := decodeJSON[struct {
				RecoveryCodes []string `json:"recoveryCodes"`
			}](t, confirmRec)
			if len(confirmed.RecoveryCodes) == 0 {
				t.Fatal("enrollment returned no recovery codes")
			}
			if rec := jar.do(t, http.MethodPost, "/api/me/security/preferences", map[string]bool{"recoveryCodeAlertEnabled": true}); rec.Code != http.StatusOK {
				t.Fatalf("enable alert status = %d, body = %s", rec.Code, rec.Body.String())
			}
			code := "not-a-valid-code"
			if tc.validProof {
				code = confirmed.RecoveryCodes[0]
			}
			rec := jar.do(t, http.MethodPost, "/api/me/security/2fa/disable", map[string]string{"code": code})
			if rec.Code != tc.wantStatus {
				t.Fatalf("disable status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if !tc.validProof {
				if got := decodeJSON[map[string]string](t, rec)["error"]; got != serviceauth.ErrInvalidTwoFactorCode.Error() {
					t.Fatalf("error = %q, want invalid two-factor code", got)
				}
			}
			summaryRec := jar.do(t, http.MethodGet, "/api/me/security", nil)
			if summaryRec.Code != http.StatusOK {
				t.Fatalf("summary status = %d, body = %s", summaryRec.Code, summaryRec.Body.String())
			}
			summary := decodeJSON[serviceauth.SecuritySummary](t, summaryRec)
			if summary.TwoFactorEnabled != !tc.validProof || summary.RecoveryCodeAlertEnabled != !tc.validProof {
				t.Fatalf("2FA or alert changed unexpectedly after disable: %+v", summary)
			}
			if !tc.validProof && summary.RecoveryCodesRemaining != len(confirmed.RecoveryCodes) {
				t.Fatalf("wrong proof consumed a recovery code: got %d, want %d", summary.RecoveryCodesRemaining, len(confirmed.RecoveryCodes))
			}
		})
	}
}
