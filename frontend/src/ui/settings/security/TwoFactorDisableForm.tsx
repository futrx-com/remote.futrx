export function TwoFactorDisableForm({
  code,
  setCode,
  onConfirm,
  onCancel,
  busy,
}: {
  code: string;
  setCode: (code: string) => void;
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
      class="space-y-2"
    >
      <label class="block space-y-1.5">
        <span class="text-xs text-ink-300">Confirm with a current code or a recovery code to disable 2FA</span>
        <input
          type="text"
          value={code}
          onInput={(event) => setCode((event.currentTarget as HTMLInputElement).value)}
          class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
        />
      </label>
      <div class="flex items-center gap-2">
        <button
          type="submit"
          disabled={busy || !code}
          class="h-9 px-2.5 rounded bg-accent-red/80 hover:bg-accent-red text-white text-[12.5px] font-medium disabled:opacity-50"
        >
          Disable
        </button>
        <button
          type="button"
          onClick={onCancel}
          class="h-9 px-2.5 rounded text-ink-300 hover:text-ink-100 hover:bg-white/[0.05] text-[12.5px]"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
