import { useState } from "preact/hooks";
import type { TwoFactorSettingsActions } from "./useSecuritySettings";

export function useTwoFactorSettingsFlow(actions: TwoFactorSettingsActions) {
  const [enrolling, setEnrolling] = useState(false);
  const [enrollmentToken, setEnrollmentToken] = useState("");
  const [secret, setSecret] = useState("");
  const [otpauthUrl, setOtpauthUrl] = useState("");
  const [confirmCode, setConfirmCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [disableCode, setDisableCode] = useState("");
  const [showDisable, setShowDisable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function startEnrollment() {
    setError(null);
    setBusy(true);
    try {
      const enrollment = await actions.beginTwoFactorEnrollment();
      setEnrollmentToken(enrollment.enrollmentToken);
      setSecret(enrollment.secret);
      setOtpauthUrl(enrollment.otpauthUrl);
      setEnrolling(true);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function confirmEnrollment() {
    setError(null);
    setBusy(true);
    try {
      const codes = await actions.confirmTwoFactorEnrollment(enrollmentToken, confirmCode);
      setRecoveryCodes(codes);
      setEnrolling(false);
      setConfirmCode("");
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function disableTwoFactor() {
    setError(null);
    setBusy(true);
    try {
      await actions.disableTwoFactor(disableCode);
      setShowDisable(false);
      setDisableCode("");
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return {
    busy,
    cancelDisable: () => setShowDisable(false),
    cancelEnrollment: () => setEnrolling(false),
    confirmCode,
    confirmEnrollment,
    disableCode,
    disableTwoFactor,
    dismissRecoveryCodes: () => setRecoveryCodes(null),
    enrolling,
    error,
    otpauthUrl,
    recoveryCodes,
    secret,
    setConfirmCode,
    setDisableCode,
    showDisable,
    showDisableForm: () => setShowDisable(true),
    startEnrollment,
  };
}
