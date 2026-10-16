import { MIN_LOCAL_PASSWORD_LENGTH } from "../../config/auth";
import { Key, Loader } from "../primitives/icons";

// The step an invited account is held at until it replaces its temporary
// password. There is deliberately no way past it other than a new password or
// going back to sign-in.
export function InitialPasswordStep({
  newPassword,
  confirmation,
  error,
  submitting,
  onNewPasswordChange,
  onConfirmationChange,
  onSubmit,
  onBack,
}: {
  newPassword: string;
  confirmation: string;
  error: string | null;
  submitting: boolean;
  onNewPasswordChange: (value: string) => void;
  onConfirmationChange: (value: string) => void;
  onSubmit: (event: Event) => void;
  onBack: () => void;
}) {
  return (
    <div class="w-full max-w-sm space-y-5 py-6">
      <div class="flex flex-col items-center gap-3 text-center">
        <div class="w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
          <Key class="w-6 h-6 text-accent-blue" />
        </div>
        <div>
          <div class="text-xl font-semibold">Choose a new password</div>
          <div class="text-xs text-ink-300 mt-1.5 leading-relaxed">
            You signed in with a temporary password. Choose your own to continue.
          </div>
        </div>
      </div>

      <form onSubmit={onSubmit} class="space-y-3">
        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">New password</span>
          <input
            type="password"
            autocomplete="new-password"
            autoFocus
            value={newPassword}
            onInput={(event) => onNewPasswordChange((event.currentTarget as HTMLInputElement).value)}
            class="w-full h-11 rounded-md bg-surface border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
          />
          <span class="block text-xs text-ink-400">
            Use at least {MIN_LOCAL_PASSWORD_LENGTH} characters, different from the temporary one.
          </span>
        </label>
        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">Confirm new password</span>
          <input
            type="password"
            autocomplete="new-password"
            value={confirmation}
            onInput={(event) => onConfirmationChange((event.currentTarget as HTMLInputElement).value)}
            class="w-full h-11 rounded-md bg-surface border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
          />
        </label>
        {error && (
          <div role="alert" class="text-xs text-accent-red bg-accent-red/10 border border-accent-red/30 rounded-lg p-3 leading-relaxed">
            {error}
          </div>
        )}
        <button
          type="submit"
          disabled={submitting}
          class="w-full h-11 rounded-md bg-accent-blue hover:bg-accent-blue/85 text-on-accent text-sm font-medium disabled:opacity-50 inline-flex items-center justify-center gap-2"
        >
          {submitting && <Loader class="w-4 h-4 animate-spin" />}
          Set password and continue
        </button>
        <button
          type="button"
          onClick={onBack}
          disabled={submitting}
          class="w-full h-10 rounded-md text-ink-300 hover:text-ink-100 hover:bg-tint-strong text-[13px] disabled:opacity-50"
        >
          Back to sign-in
        </button>
      </form>
    </div>
  );
}
