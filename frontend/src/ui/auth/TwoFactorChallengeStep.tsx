import { Loader, ShieldCheck } from "../primitives/icons";

export function TwoFactorChallengeStep({
  code,
  error,
  submitting,
  onCodeChange,
  onSubmit,
  onCancel,
}: {
  code: string;
  error: string | null;
  submitting: boolean;
  onCodeChange: (value: string) => void;
  onSubmit: (event: Event) => void;
  onCancel: () => void;
}) {
  return (
    <div class="w-full max-w-sm space-y-5 py-6">
      <div class="flex flex-col items-center gap-3 text-center">
        <div class="w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
          <ShieldCheck class="w-6 h-6 text-accent-blue" />
        </div>
        <div>
          <div class="text-xl font-semibold">Two-factor authentication</div>
          <div class="text-xs text-ink-300 mt-1.5 leading-relaxed">
            Enter the 6-digit code from your authenticator app, or one of your recovery codes.
          </div>
        </div>
      </div>

      <form onSubmit={onSubmit} class="space-y-3">
        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">Code</span>
          <input
            type="text"
            inputMode="text"
            autocomplete="one-time-code"
            autoFocus
            value={code}
            onInput={(event) => onCodeChange((event.currentTarget as HTMLInputElement).value)}
            class="w-full h-11 rounded-md bg-[#101318] border border-white/10 px-3 text-sm text-ink-100 tracking-widest focus:outline-none focus:border-accent-blue"
          />
        </label>
        {error && (
          <div class="text-xs text-accent-red bg-accent-red/10 border border-accent-red/30 rounded-lg p-3 leading-relaxed">
            {error}
          </div>
        )}
        <button
          type="submit"
          disabled={submitting}
          class="w-full h-11 rounded-md bg-accent-blue hover:bg-accent-blue/85 text-white text-sm font-medium disabled:opacity-50 inline-flex items-center justify-center gap-2"
        >
          {submitting && <Loader class="w-4 h-4 animate-spin" />}
          Verify
        </button>
        <button
          type="button"
          onClick={onCancel}
          class="w-full h-10 rounded-md text-ink-300 hover:text-ink-100 hover:bg-white/[0.05] text-[13px]"
        >
          Cancel and sign in again
        </button>
      </form>
    </div>
  );
}
