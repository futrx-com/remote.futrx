import { useState } from "preact/hooks";
import type { DirectoryUser, UserRole } from "../../models/user";
import { inviteFormState } from "../../state/hooks/users/inviteFormState";
import { Check, X } from "../primitives/icons";

export function UserRow({
  user,
  isSelf,
  canResetPassword,
  smtpConfigured,
  onRemove,
  onSetRole,
  onResetPassword,
}: {
  user: DirectoryUser;
  isSelf: boolean;
  /** False for yourself and for the local administrator. */
  canResetPassword: boolean;
  smtpConfigured: boolean;
  onRemove: () => Promise<void>;
  onSetRole: (role: UserRole) => Promise<void>;
  onResetPassword: (sendEmail: boolean) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [resetOpen, setResetOpen] = useState(false);
  const [sendEmailChoice, setSendEmailChoice] = useState<boolean | null>(null);
  const resetView = inviteFormState.view(
    { googleConfigured: false, smtpConfigured },
    { method: "password", sendEmail: sendEmailChoice }
  );

  const resetPassword = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onResetPassword(resetView.sendEmail);
      setResetOpen(false);
      setSendEmailChoice(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const toggleRole = async () => {
    const next: UserRole = user.role === "admin" ? "member" : "admin";
    setBusy(true);
    setErr(null);
    try {
      await onSetRole(next);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onRemove();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 min-w-0">
        <span class="text-[12.5px] text-ink-50 truncate" title={user.email}>
          {user.email}
        </span>
        <span
          class={`inline-flex items-center h-5 px-1.5 rounded text-[11px] font-medium ${
            user.role === "admin"
              ? "text-accent-blue bg-accent-blue/[0.14]"
              : "text-ink-300 bg-tint"
          }`}
        >
          {user.role}
        </span>
        {isSelf && (
          <span class="inline-flex items-center h-5 px-1.5 rounded text-[11px] text-accent-green bg-accent-green/[0.10]">
            <Check class="w-3 h-3 mr-1" /> you
          </span>
        )}
        {user.passwordChangePending && (
          <span class="inline-flex items-center h-5 px-1.5 rounded text-[11px] text-accent-yellow bg-accent-yellow/[0.10]">
            Password change pending
          </span>
        )}
        <div class="ml-auto flex items-center gap-1">
          {canResetPassword && (
            <button
              type="button"
              onClick={() => setResetOpen((open) => !open)}
              disabled={busy}
              aria-expanded={resetOpen}
              class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong disabled:opacity-50"
              title="Reset temporary password"
            >
              reset password
            </button>
          )}
          <button
            type="button"
            onClick={toggleRole}
            disabled={busy}
            class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong disabled:opacity-50"
            title={user.role === "admin" ? "Demote to member" : "Promote to admin"}
          >
            {user.role === "admin" ? "demote" : "promote"}
          </button>
          <button
            type="button"
            onClick={remove}
            disabled={busy}
            class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
            aria-label={`Remove ${user.email}`}
            title="Remove user"
          >
            <X class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
      {resetOpen && canResetPassword && (
        <div class="rounded border border-line bg-inset p-2.5 space-y-2">
          <div class="text-[12px] text-ink-300 leading-snug">
            Issue a new temporary password for {user.email}. Their current password and every
            session they have open stop working, and they choose a new password at next sign-in.
          </div>
          <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[12px] text-ink-300">
            {resetView.showEmailOption && (
              <label class="inline-flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={resetView.sendEmail}
                  onChange={(e) => setSendEmailChoice((e.target as HTMLInputElement).checked)}
                />
                <span>Email the credentials to the user</span>
              </label>
            )}
            {resetView.emailHint && <span class="text-ink-400">{resetView.emailHint}</span>}
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              onClick={resetPassword}
              disabled={busy}
              class="btn btn-primary btn-sm disabled:opacity-50"
            >
              {busy ? "Resetting…" : "Reset temporary password"}
            </button>
            <button
              type="button"
              onClick={() => setResetOpen(false)}
              disabled={busy}
              class="h-8 px-2.5 rounded text-[12px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong disabled:opacity-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </div>
  );
}
