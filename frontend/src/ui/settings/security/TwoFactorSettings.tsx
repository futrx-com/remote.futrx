import type { SecuritySettingsController } from "../../../state/hooks/auth/useSecuritySettings";
import { useTwoFactorSettingsFlow } from "../../../state/hooks/auth/useTwoFactorSettingsFlow";
import { Check, Key, Loader, ShieldCheck } from "../../primitives/icons";
import { QrCode } from "../../primitives/QrCode";

export function TwoFactorSettings({ controller }: { controller: SecuritySettingsController }) {
  const { settings } = controller;
  const flow = useTwoFactorSettingsFlow(controller);

  const twoFactorEnabled = settings?.twoFactorEnabled ?? false;

  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
        <div class="h-9 w-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none">
          <ShieldCheck class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="text-[14.5px] font-semibold text-ink-50">Two-factor authentication</div>
            {twoFactorEnabled ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="w-3.5 h-3.5" /> enabled
              </span>
            ) : (
              <span class="text-[11px] text-ink-400">not enabled</span>
            )}
          </div>
          <div class="text-[12px] text-ink-300 mt-1 leading-relaxed">
            Require a code from an authenticator app (or a recovery code) to sign in.
          </div>
        </div>
      </header>

      <div class="p-3.5 space-y-3">
        {flow.recoveryCodes && (
          <div class="rounded-md border border-accent-yellow/30 bg-accent-yellow/[0.08] p-3 space-y-2">
            <div class="text-[13px] font-medium text-ink-50">
              Save these recovery codes now — they won't be shown again.
            </div>
            <div class="grid grid-cols-2 gap-1.5 font-mono text-[12.5px] text-ink-100">
              {flow.recoveryCodes.map((code) => (
                <div key={code} class="bg-black/30 border border-white/10 rounded px-2 py-1">
                  {code}
                </div>
              ))}
            </div>
            <button
              type="button"
              onClick={flow.dismissRecoveryCodes}
              class="h-8 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12px] font-medium"
            >
              I've saved these codes
            </button>
          </div>
        )}

        {!twoFactorEnabled && !flow.enrolling && !flow.recoveryCodes && (
          <button
            type="button"
            disabled={flow.busy}
            onClick={() => void flow.startEnrollment()}
            class="h-10 px-3 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center gap-2"
          >
            {flow.busy && <Loader class="w-3.5 h-3.5 animate-spin" />}
            Set up two-factor authentication
          </button>
        )}

        {flow.enrolling && (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void flow.confirmEnrollment();
            }}
            class="space-y-3"
          >
            <div class="flex flex-col sm:flex-row gap-3">
              <QrCode value={flow.otpauthUrl} size={160} class="rounded bg-white p-2 flex-none" />
              <div class="flex-1 min-w-0 space-y-1.5">
                <div class="text-[12px] text-ink-300">
                  Scan with your authenticator app, or enter this secret manually:
                </div>
                <code class="block break-all rounded bg-black/30 border border-white/10 px-2.5 py-2 text-[11.5px] text-ink-100">
                  {flow.secret}
                </code>
              </div>
            </div>
            <label class="block space-y-1.5">
              <span class="text-xs text-ink-300">Code from your authenticator app</span>
              <input
                type="text"
                inputMode="numeric"
                value={flow.confirmCode}
                onInput={(event) => flow.setConfirmCode((event.currentTarget as HTMLInputElement).value)}
                class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
              />
            </label>
            <div class="flex items-center gap-2">
              <button
                type="submit"
                disabled={flow.busy}
                class="h-10 px-3 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center gap-2"
              >
                {flow.busy && <Loader class="w-3.5 h-3.5 animate-spin" />}
                Confirm
              </button>
              <button
                type="button"
                onClick={flow.cancelEnrollment}
                class="h-10 px-3 rounded-md text-ink-300 hover:text-ink-100 hover:bg-white/[0.05] text-[13px]"
              >
                Cancel
              </button>
            </div>
          </form>
        )}

        {twoFactorEnabled && !flow.showDisable && (
          <div class="flex items-center gap-2">
            <span class="text-[12.5px] text-ink-300">
              {settings?.recoveryCodesRemaining ?? 0} recovery codes remaining.
            </span>
            <button
              type="button"
              onClick={flow.showDisableForm}
              class="h-9 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12.5px] font-medium inline-flex items-center gap-1.5"
            >
              <Key class="w-3.5 h-3.5" /> Disable two-factor authentication
            </button>
          </div>
        )}

        {flow.showDisable && (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void flow.disableTwoFactor();
            }}
            class="space-y-2"
          >
            <label class="block space-y-1.5">
              <span class="text-xs text-ink-300">Confirm with a current code or a recovery code</span>
              <input
                type="text"
                value={flow.disableCode}
                onInput={(event) => flow.setDisableCode((event.currentTarget as HTMLInputElement).value)}
                class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
              />
            </label>
            <div class="flex items-center gap-2">
              <button
                type="submit"
                disabled={flow.busy}
                class="h-9 px-2.5 rounded bg-accent-red/80 hover:bg-accent-red text-white text-[12.5px] font-medium disabled:opacity-50"
              >
                Disable
              </button>
              <button
                type="button"
                onClick={flow.cancelDisable}
                class="h-9 px-2.5 rounded text-ink-300 hover:text-ink-100 hover:bg-white/[0.05] text-[12.5px]"
              >
                Cancel
              </button>
            </div>
          </form>
        )}

        {flow.error && <div class="text-xs text-accent-red">{flow.error}</div>}
      </div>
    </section>
  );
}
