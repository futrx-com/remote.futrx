package auth

import (
	"context"
	"errors"
)

// ErrRecoveryCodeAlertRequiresTwoFactor rejects an alert preference that
// cannot fire until the account has recovery codes.
var ErrRecoveryCodeAlertRequiresTwoFactor = errors.New("enable two-factor authentication before turning on the recovery-code alert")

// TwoFactorEnrollmentCompletion is the outcome of confirming enrollment for
// an authenticated account. SessionCookieValue is set only when an existing
// account preference requires the current browser session to be tracked.
type TwoFactorEnrollmentCompletion struct {
	RecoveryCodes      []string
	SessionCookieValue string
}

// SecurityPreferencesUpdate is a partial account-security preference change.
// Nil fields retain their current value.
type SecurityPreferencesUpdate struct {
	SingleSessionEnabled     *bool `json:"singleSessionEnabled"`
	HistoryEnabled           *bool `json:"historyEnabled"`
	RecoveryCodeAlertEnabled *bool `json:"recoveryCodeAlertEnabled"`
}

// SecurityPreferencesUpdateResult carries the refreshed view and any session
// cookie reissued while applying the change.
type SecurityPreferencesUpdateResult struct {
	Summary            SecuritySummary
	SessionCookieValue string
}

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

// CompleteTwoFactorEnrollment confirms enrollment and keeps an already
// tracked browser session current. Preference lookup and session reissue are
// intentionally best-effort so successful enrollment still returns its
// one-time recovery codes when that follow-up work fails.
func (s *Service) CompleteTwoFactorEnrollment(
	ctx context.Context,
	user User,
	enrollmentToken, code, ip, userAgent string,
) (TwoFactorEnrollmentCompletion, error) {
	recoveryCodes, email, err := s.ConfirmTwoFactorEnrollment(ctx, user.Email, enrollmentToken, code)
	if err != nil {
		return TwoFactorEnrollmentCompletion{}, err
	}

	result := TwoFactorEnrollmentCompletion{RecoveryCodes: recoveryCodes}
	if prefs, err := s.SecurityPreferences(ctx, email); err == nil {
		if prefs.SingleSessionEnabled || prefs.HistoryEnabled || prefs.RecoveryCodeAlertEnabled {
			if cookieValue, err := s.ReissueTrackedSession(ctx, user, ip, userAgent); err == nil {
				result.SessionCookieValue = cookieValue
			}
		}
	}
	return result, nil
}

// DisableTwoFactor removes email's 2FA enrollment after verifying proof of
// possession; see twoFactorAuthenticator.Disable.
func (s *Service) DisableTwoFactor(ctx context.Context, email, code string) error {
	return s.twoFactor.Disable(ctx, email, code)
}

// CompleteTwoFactorDisable disables 2FA for an authenticated account, then
// clears the recovery-code alert preference and any pending alert because both
// are meaningful only while 2FA is on. Cleanup remains best-effort after the
// primary disable succeeds.
func (s *Service) CompleteTwoFactorDisable(ctx context.Context, email, code string) error {
	if err := s.DisableTwoFactor(ctx, email, code); err != nil {
		return err
	}
	if prefs, err := s.SecurityPreferences(ctx, email); err == nil && prefs.RecoveryCodeAlertEnabled {
		prefs.RecoveryCodeAlertEnabled = false
		_ = s.SetSecurityPreferences(ctx, email, prefs)
	}
	_ = s.AckSecurityAlert(ctx, email)
	return nil
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

// UpdateSecurityPreferences merges a partial preference change, enforces the
// recovery-alert dependency on 2FA, and makes a newly single-session account's
// current browser active immediately. Session reissue remains best-effort.
func (s *Service) UpdateSecurityPreferences(
	ctx context.Context,
	user User,
	update SecurityPreferencesUpdate,
	ip, userAgent string,
) (SecurityPreferencesUpdateResult, error) {
	prefs, err := s.SecurityPreferences(ctx, user.Email)
	if err != nil {
		return SecurityPreferencesUpdateResult{}, err
	}

	turningSingleSessionOn := false
	if update.SingleSessionEnabled != nil {
		if *update.SingleSessionEnabled && !prefs.SingleSessionEnabled {
			turningSingleSessionOn = true
		}
		prefs.SingleSessionEnabled = *update.SingleSessionEnabled
	}
	if update.HistoryEnabled != nil {
		prefs.HistoryEnabled = *update.HistoryEnabled
	}
	if update.RecoveryCodeAlertEnabled != nil {
		if *update.RecoveryCodeAlertEnabled && !s.TwoFactorEnabled(ctx, user.Email) {
			return SecurityPreferencesUpdateResult{}, ErrRecoveryCodeAlertRequiresTwoFactor
		}
		prefs.RecoveryCodeAlertEnabled = *update.RecoveryCodeAlertEnabled
	}

	if err := s.SetSecurityPreferences(ctx, user.Email, prefs); err != nil {
		return SecurityPreferencesUpdateResult{}, err
	}

	result := SecurityPreferencesUpdateResult{}
	if turningSingleSessionOn {
		if cookieValue, err := s.ReissueTrackedSession(ctx, user, ip, userAgent); err == nil {
			result.SessionCookieValue = cookieValue
		}
	}

	result.Summary, err = s.SecuritySummary(ctx, user.Email)
	return result, err
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
