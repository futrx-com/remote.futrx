import type { SecuritySettingsController } from "../../../state/hooks/auth/useSecuritySettings";
import { useTwoFactorSettingsFlow } from "../../../state/hooks/auth/useTwoFactorSettingsFlow";
import { Check, Key, Loader, ShieldCheck } from "../../primitives/icons";
import { TwoFactorRecoveryCodes } from "./TwoFactorRecoveryCodes";
import { TwoFactorEnrollForm } from "./TwoFactorEnrollForm";
import { TwoFactorDisableForm } from "./TwoFactorDisableForm";
import { TwoFactorRegenerateForm } from "./TwoFactorRegenerateForm";

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
          <TwoFactorRecoveryCodes
            codes={flow.recoveryCodes}
            onDismiss={flow.dismissRecoveryCodes}
          />
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
          <TwoFactorEnrollForm
            otpauthUrl={flow.otpauthUrl}
            secret={flow.secret}
            confirmCode={flow.confirmCode}
            setConfirmCode={flow.setConfirmCode}
            onConfirm={() => void flow.confirmEnrollment()}
            onCancel={flow.cancelEnrollment}
            busy={flow.busy}
          />
        )}

        {twoFactorEnabled && !flow.showDisable && !flow.showRegenerate && (
          <div class="flex items-center gap-2">
            <span class="text-[12.5px] text-ink-300">
              {settings?.recoveryCodesRemaining ?? 0} recovery codes remaining.
            </span>
            <button
              type="button"
              onClick={flow.showRegenerateForm}
              class="h-9 px-2.5 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[12.5px] font-medium inline-flex items-center gap-1.5"
            >
              Regenerate codes
            </button>
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
          <TwoFactorDisableForm
            code={flow.disableCode}
            setCode={flow.setDisableCode}
            onConfirm={() => void flow.disableTwoFactor()}
            onCancel={flow.cancelDisable}
            busy={flow.busy}
          />
        )}

        {flow.showRegenerate && (
          <TwoFactorRegenerateForm
            code={flow.regenerateCode}
            setCode={flow.setRegenerateCode}
            onConfirm={() => void flow.regenerateRecoveryCodes()}
            onCancel={flow.cancelRegenerate}
            busy={flow.busy}
          />
        )}

        {flow.error && <div class="text-xs text-accent-red">{flow.error}</div>}
      </div>
    </section>
  );
}
