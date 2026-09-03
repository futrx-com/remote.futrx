import { useState } from "preact/hooks";
import { localAuthApi } from "../../../api/authApi";
import type { LoginMode } from "../../../models/auth";
import { localAuthFormState } from "./localAuthFormState";
import { returnUrlPolicy } from "./returnUrlPolicy";
import { usePendingTwoFactorChallenge } from "./usePendingTwoFactorChallenge";

interface LocalAuthControllerOptions {
  mode: LoginMode;
  adminEmail: string;
  onSuccess: () => Promise<void>;
}

export function useLocalAuthController({
  mode,
  adminEmail,
  onSuccess,
}: LocalAuthControllerOptions) {
  ////////////////
  // Local State
  ////////////////
  const [email, setEmail] = useState(mode === "legacy-setup" ? adminEmail : "");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const setup = localAuthFormState.isSetup(mode);

  ////////////////
  // Global State
  ////////////////
  const params = new URLSearchParams(location.search);
  const oauthError = params.get("error");
  const errorEmail = params.get("email") ?? "";
  const returnTo = returnUrlPolicy.safeTarget(params.get("return_to") ?? "", location.origin);

  // A password or Google login can come back asking for a second factor
  // instead of completing outright. The Google callback signals the same
  // thing via a `?twoFactorRequired=1` redirect, since it has no JSON
  // response to branch on.
  const twoFactorChallenge = usePendingTwoFactorChallenge({
    initiallyPending: mode === "login" && params.get("twoFactorRequired") === "1",
    onVerified: completeLogin,
  });

  ////////////////
  // Handlers
  ////////////////
  async function submit(event: Event) {
    event.preventDefault();
    const submission = localAuthFormState.prepareSubmission({
      mode,
      email,
      password,
      confirmation,
    });
    if (!submission.valid) {
      setError(submission.error);
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      if (setup) {
        await localAuthApi.claim(submission.email, password);
        await onSuccess();
        return;
      }
      const result = await localAuthApi.login(submission.email, password);
      if (result.twoFactorRequired) {
        twoFactorChallenge.begin();
        return;
      }
      await completeLogin();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  async function completeLogin() {
    await onSuccess();
    if (returnTo) location.assign(returnTo);
  }

  return {
    confirmation,
    email,
    error,
    errorEmail,
    googleURL: `/auth/google/login${returnTo ? `?return_to=${encodeURIComponent(returnTo)}` : ""}`,
    oauthError,
    password,
    setConfirmation,
    setEmail,
    setPassword,
    setup,
    submit,
    submitting,
    challenge: {
      cancel: twoFactorChallenge.cancel,
      code: twoFactorChallenge.code,
      error: twoFactorChallenge.error,
      pending: twoFactorChallenge.pending,
      setCode: twoFactorChallenge.setCode,
      submit: twoFactorChallenge.submit,
      submitting: twoFactorChallenge.submitting,
    },
  };
}
