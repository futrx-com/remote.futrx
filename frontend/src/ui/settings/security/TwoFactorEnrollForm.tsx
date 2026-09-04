import { QrCode } from "../../primitives/QrCode";
import { Loader } from "../../primitives/icons";

export function TwoFactorEnrollForm({
  otpauthUrl,
  secret,
  confirmCode,
  setConfirmCode,
  onConfirm,
  onCancel,
  busy,
}: {
  otpauthUrl: string;
  secret: string;
  confirmCode: string;
  setConfirmCode: (code: string) => void;
  onConfirm: () => void;
  onCancel: () => void;
  busy: boolean;
}) {
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onConfirm();
      }}
      class="space-y-3"
    >
      <div class="flex flex-col sm:flex-row gap-3">
        <QrCode value={otpauthUrl} size={160} class="rounded bg-white p-2 flex-none" />
        <div class="flex-1 min-w-0 space-y-1.5">
          <div class="text-[12px] text-ink-300">
            Scan with your authenticator app, or enter this secret manually:
          </div>
          <code class="block break-all rounded bg-black/30 border border-white/10 px-2.5 py-2 text-[11.5px] text-ink-100">
            {secret}
          </code>
        </div>
      </div>
      <label class="block space-y-1.5">
        <span class="text-xs text-ink-300">Code from your authenticator app</span>
        <input
          type="text"
          inputMode="numeric"
          value={confirmCode}
          onInput={(event) => setConfirmCode((event.currentTarget as HTMLInputElement).value)}
          class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
        />
      </label>
      <div class="flex items-center gap-2">
        <button
          type="submit"
          disabled={busy}
          class="h-10 px-3 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center gap-2"
        >
          {busy && <Loader class="w-3.5 h-3.5 animate-spin" />}
          Confirm
        </button>
        <button
          type="button"
          onClick={onCancel}
          class="h-10 px-3 rounded-md text-ink-300 hover:text-ink-100 hover:bg-white/[0.05] text-[13px]"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
