import { useState } from "preact/hooks";
import type { InviteInput, InviteMethod, UserRole } from "../../models/user";
import { inviteFormState } from "../../state/hooks/users/inviteFormState";

const METHOD_LABEL: Record<InviteMethod, string> = {
  google: "Sign in with Google",
  password: "Email and password",
};

export function AddUserForm({
  googleConfigured,
  smtpConfigured,
  onAdd,
}: {
  googleConfigured: boolean;
  smtpConfigured: boolean;
  onAdd: (input: InviteInput) => Promise<unknown>;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<UserRole>("member");
  const [methodChoice, setMethodChoice] = useState<InviteMethod | null>(null);
  const [sendEmailChoice, setSendEmailChoice] = useState<boolean | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const view = inviteFormState.view(
    { googleConfigured, smtpConfigured },
    { method: methodChoice, sendEmail: sendEmailChoice }
  );

  const submit = async (e: Event) => {
    e.preventDefault();
    const em = email.trim().toLowerCase();
    if (!em) {
      setErr("Email is required.");
      return;
    }
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(em)) {
      setErr("That doesn't look like an email.");
      return;
    }
    setErr(null);
    setSubmitting(true);
    try {
      await onAdd({ email: em, role, method: view.method, sendEmail: view.sendEmail });
      setEmail("");
      setRole("member");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form
      onSubmit={submit}
      class="rounded-md border border-line bg-tint p-2.5 space-y-2"
    >
      <div class="grid gap-2 sm:grid-cols-[2fr_auto_auto] items-center">
        <input
          type="email"
          value={email}
          onInput={(e) => setEmail((e.target as HTMLInputElement).value)}
          placeholder="someone@example.com"
          spellcheck={false}
          autoComplete="off"
          class="h-9 px-2.5 rounded border border-line bg-inset text-[13px] text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
        />
        <select
          value={role}
          onChange={(e) => setRole((e.target as HTMLSelectElement).value as UserRole)}
          aria-label="Role"
          class="h-9 px-2 rounded border border-line bg-inset text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
        >
          <option value="member">member</option>
          <option value="admin">admin</option>
        </select>
        <button
          type="submit"
          disabled={submitting}
          class="btn btn-primary btn-sm disabled:opacity-50"
        >
          {submitting ? "Adding…" : "Add"}
        </button>
      </div>
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[12px] text-ink-300">
        {view.methods.length > 1 && (
          <label class="inline-flex items-center gap-2">
            <span>Sign-in method</span>
            <select
              value={view.method}
              onChange={(e) => setMethodChoice((e.target as HTMLSelectElement).value as InviteMethod)}
              class="h-8 px-2 rounded border border-line bg-inset text-[12.5px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
            >
              {view.methods.map((method) => (
                <option key={method} value={method}>
                  {METHOD_LABEL[method]}
                </option>
              ))}
            </select>
          </label>
        )}
        {view.showEmailOption && (
          <label class="inline-flex items-center gap-2">
            <input
              type="checkbox"
              checked={view.sendEmail}
              onChange={(e) => setSendEmailChoice((e.target as HTMLInputElement).checked)}
            />
            <span>Email the credentials to the user</span>
          </label>
        )}
        {view.emailHint && <span class="text-ink-400">{view.emailHint}</span>}
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </form>
  );
}
