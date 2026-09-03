import { useState } from "preact/hooks";
import { twoFactorApi } from "../../../api/authApi";

interface PendingTwoFactorChallengeOptions {
  initiallyPending: boolean;
  onVerified: () => Promise<void>;
}

export function usePendingTwoFactorChallenge({
  initiallyPending,
  onVerified,
}: PendingTwoFactorChallengeOptions) {
  const [pending, setPending] = useState(initiallyPending);
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: Event) {
    event.preventDefault();
    if (!code.trim()) {
      setError("Enter a code from your authenticator app or a recovery code.");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      await twoFactorApi.verify(code.trim());
      setPending(false);
      await onVerified();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  async function cancel() {
    setCode("");
    setError(null);
    setPending(false);
    try {
      await twoFactorApi.cancel();
    } catch {
      // Best-effort: the pending cookie also just expires on its own.
    }
  }

  return {
    begin: () => setPending(true),
    cancel,
    code,
    error,
    pending,
    setCode,
    submit,
    submitting,
  };
}
