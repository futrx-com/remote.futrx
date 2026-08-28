package auth

import "context"

// TwoFactorEnabled reports whether email has completed TOTP enrollment.
func (s *Service) TwoFactorEnabled(ctx context.Context, email string) bool {
	return s.twoFactor.Enabled(ctx, email)
}

// BeginTwoFactorEnrollment starts TOTP enrollment for email; see
// twoFactorAuthenticator.BeginEnrollment.
func (s *Service) BeginTwoFactorEnrollment(ctx context.Context, email string) (enrollmentToken, secretBase32, otpauthURL string, err error) {
	return s.twoFactor.BeginEnrollment(ctx, email)
}

// ConfirmTwoFactorEnrollment completes TOTP enrollment; see
// twoFactorAuthenticator.ConfirmEnrollment.
func (s *Service) ConfirmTwoFactorEnrollment(ctx context.Context, expectedEmail, enrollmentToken, code string) (recoveryCodes []string, email string, err error) {
	return s.twoFactor.ConfirmEnrollment(ctx, expectedEmail, enrollmentToken, code)
}

// DisableTwoFactor removes email's 2FA enrollment after verifying proof of
// possession; see twoFactorAuthenticator.Disable.
func (s *Service) DisableTwoFactor(ctx context.Context, email, code string) error {
	return s.twoFactor.Disable(ctx, email, code)
}

// RegenerateRecoveryCodes replaces email's recovery codes; see
// twoFactorAuthenticator.RegenerateRecoveryCodes.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, email, code string) ([]string, error) {
	return s.twoFactor.RegenerateRecoveryCodes(ctx, email, code)
}

// SecurityPreferences returns email's current SecurityPreferences.
func (s *Service) SecurityPreferences(ctx context.Context, email string) (SecurityPreferences, error) {
	return s.registry.Preferences(ctx, email)
}

// SetSecurityPreferences overwrites email's SecurityPreferences.
func (s *Service) SetSecurityPreferences(ctx context.Context, email string, prefs SecurityPreferences) error {
	return s.registry.SetPreferences(ctx, email, prefs)
}

// AckSecurityAlert clears email's pending SecurityAlert, if any.
func (s *Service) AckSecurityAlert(ctx context.Context, email string) error {
	return s.registry.AckAlert(ctx, email)
}

// SecuritySummary aggregates 2FA status, SecurityPreferences, sign-in
// history, and any pending alert for the Security settings tab.
func (s *Service) SecuritySummary(ctx context.Context, email string) (SecuritySummary, error) {
	prefs, err := s.registry.Preferences(ctx, email)
	if err != nil {
		return SecuritySummary{}, err
	}
	history, err := s.registry.History(ctx, email)
	if err != nil {
		return SecuritySummary{}, err
	}
	alert, err := s.registry.PendingAlert(ctx, email)
	if err != nil {
		return SecuritySummary{}, err
	}
	return SecuritySummary{
		TwoFactorEnabled:         s.twoFactor.Enabled(ctx, email),
		RecoveryCodesRemaining:   s.twoFactor.RecoveryCodesRemaining(ctx, email),
		SingleSessionEnabled:     prefs.SingleSessionEnabled,
		HistoryEnabled:           prefs.HistoryEnabled,
		RecoveryCodeAlertEnabled: prefs.RecoveryCodeAlertEnabled,
		Sessions:                 history.Entries,
		SecurityAlert:            alert,
	}, nil
}

// ReissueTrackedSession re-signs a new session for an already-authenticated
// user, going through the same IssueSession path a fresh login would. Used
// right after a Security-tab change (enabling 2FA, single-session, etc.) so
// the browser that just made the change is immediately recognized as the
// account's active/tracked session, instead of waiting for its next login.
// The SignInMethod recorded is inferred from the session's Sub (local-admin
// implies password; anything else implies Google) since this isn't a fresh
// credential check.
func (s *Service) ReissueTrackedSession(ctx context.Context, user User, ip, userAgent string) (string, error) {
	method := SignInMethodGoogle
	if user.Sub == "local-admin" {
		method = SignInMethodPassword
	}
	return s.IssueSession(ctx, user, method, ip, userAgent)
}
