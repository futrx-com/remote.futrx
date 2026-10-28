import { useLayoutEffect, useRef, useState } from "preact/hooks";
import type { PasswordChangeInput } from "../../../models/security";
import { passwordChangeFormState } from "../../../state/hooks/auth/passwordChangeFormState";
import { MIN_LOCAL_PASSWORD_LENGTH } from "../../../config/auth";

export function PasswordChangeForm({
  twoFactorEnabled,
  changePassword,
}: {
  twoFactorEnabled: boolean;
  changePassword: (input: PasswordChangeInput) => Promise<void>;
}) {
  // Keep credentials in the form only, outside shared or persisted state.
  const formRef = useRef<HTMLFormElement>(null);
  const submitting = useRef(false);
  const mounted = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  useLayoutEffect(() => {
    mounted.current = true;
    const form = formRef.current;
    return () => {
      mounted.current = false;
      // Capture the node before Preact clears its ref; wipe detached nodes too.
      form?.reset();
      form?.querySelectorAll("input").forEach((input) => { input.value = ""; });
    };
  }, []);

  async function submit(form: HTMLFormElement) {
    if (submitting.current) return;
    setError(null);
    setSuccess(false);
    const fields = new FormData(form);
    const submission = passwordChangeFormState.prepareSubmission({
      currentPassword: String(fields.get("currentPassword") ?? ""),
      newPassword: String(fields.get("newPassword") ?? ""),
      confirmation: String(fields.get("confirmation") ?? ""),
      code: String(fields.get("code") ?? ""),
      twoFactorEnabled,
    });
    if (!submission.valid) {
      setError(submission.error);
      return;
    }
    submitting.current = true;
    setBusy(true);
    try {
      await changePassword(submission.input);
      form.reset();
      if (mounted.current) setSuccess(true);
    } catch (cause) {
      if (mounted.current) {
        setError(cause instanceof Error ? cause.message : "Could not change password.");
      }
    } finally {
      submitting.current = false;
      if (mounted.current) setBusy(false);
    }
  }

  return (
    <section class="rounded-lg border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 border-b border-line">
        <h3 class="text-[14.5px] font-semibold text-ink-50">Password</h3>
        <p class="text-[12px] text-ink-300 mt-1 leading-relaxed">
          Changing your password signs out your other sessions. This browser stays signed in.
        </p>
      </header>
      <form
        ref={formRef}
        noValidate
        aria-busy={busy}
        onSubmit={(event) => {
          event.preventDefault();
          void submit(event.currentTarget);
        }}
        class="p-3.5 space-y-3"
      >
        <fieldset disabled={busy} class="space-y-3">
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Current password</span>
            <input name="currentPassword" type="password" autoComplete="current-password" required class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue" />
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">New password</span>
            <input name="newPassword" type="password" autoComplete="new-password" required class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue" />
            <span class="block text-xs text-ink-400">Use at least {MIN_LOCAL_PASSWORD_LENGTH} characters.</span>
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Confirm new password</span>
            <input name="confirmation" type="password" autoComplete="new-password" required class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue" />
          </label>
          {twoFactorEnabled && (
            <label class="block space-y-1.5">
              <span class="text-xs text-ink-300">Authenticator code or recovery code</span>
              <input name="code" type="text" autoComplete="one-time-code" required class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue" />
            </label>
          )}
          <button type="submit" disabled={busy} class="h-10 px-3 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-on-accent text-[13px] font-medium disabled:opacity-50">
            {busy ? "Changing password…" : "Change password"}
          </button>
        </fieldset>
        {error && <div role="alert" class="text-xs text-accent-red">{error}</div>}
        {success && <div role="status" class="text-xs text-accent-green">Password changed. Your other sessions were signed out.</div>}
      </form>
    </section>
  );
}
