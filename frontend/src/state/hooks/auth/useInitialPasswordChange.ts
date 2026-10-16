import { useState } from "preact/hooks";
import { initialPasswordApi } from "../../../api/authApi";
import { ApiError } from "../../../api/apiError";
import type { InitialPasswordResult } from "../../../models/auth";
import { initialPasswordFormState } from "./initialPasswordFormState";

interface InitialPasswordChangeOptions {
  initiallyPending: boolean;
  /** The new password was accepted and a session now exists. */
  onSignedIn: () => Promise<void>;
  /** The account has 2FA: the second factor is asked for next. */
  onTwoFactorRequired: () => void;
  /** The server no longer honours this step; the user starts again at sign-in. */
  onExpired: (message: string) => void;
}

/**
 * The set-password step an invited account is held at after signing in with a
 * temporary password. The typed passwords exist only in this hook's state and
 * are cleared when the step ends, whichever way it ends.
 */
export function useInitialPasswordChange({
  initiallyPending,
  onSignedIn,
  onTwoFactorRequired,
  onExpired,
}: InitialPasswordChangeOptions) {
  const [pending, setPending] = useState(initiallyPending);
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function finish() {
    setNewPassword("");
    setConfirmation("");
    setError(null);
    setPending(false);
  }

  async function submit(event: Event) {
    event.preventDefault();
    const submission = initialPasswordFormState.prepareSubmission({ newPassword, confirmation });
    if (!submission.valid) {
      setError(submission.error);
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const result: InitialPasswordResult = await initialPasswordApi.submit(submission.newPassword);
      finish();
      if (result.outcome === "two-factor") {
        onTwoFactorRequired();
        return;
      }
      await onSignedIn();
    } catch (cause) {
      const failure = initialPasswordFormState.classifyFailure(
        cause instanceof ApiError ? cause.status : undefined,
        cause instanceof Error ? cause.message : "",
      );
      if (failure.kind === "expired") {
        finish();
        onExpired(failure.message);
      } else {
        setError(failure.message);
      }
    } finally {
      setSubmitting(false);
    }
  }

  async function cancel() {
    finish();
    try {
      await initialPasswordApi.cancel();
    } catch {
      // Best-effort: the change cookie also just expires on its own.
    }
  }

  return {
    begin: () => setPending(true),
    cancel,
    confirmation,
    error,
    newPassword,
    pending,
    setConfirmation,
    setNewPassword,
    submit,
    submitting,
  };
}
