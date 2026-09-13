import { useEmailSettingsController } from "../../../state/hooks/server/useEmailSettingsController";
import { Check, Loader, Mail } from "../../primitives/icons";
import { EmailProviderSelector } from "./EmailProviderSelector";
import { SMTPConnectionFields } from "./SMTPConnectionFields";
import { SMTPTestPanel } from "./SMTPTestPanel";

export function EmailSettings({ defaultRecipient }: { defaultRecipient: string }) {
  const {
    configured,
    error,
    form,
    loading,
    remove,
    save,
    saving,
    selectProvider,
    sendTest,
    setAuthentication,
    setFromAddress,
    setHost,
    setPassword,
    setPort,
    setTlsMode,
    setUsername,
    testMessage,
    testing,
  } = useEmailSettingsController();

  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
          <Mail class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="text-[14.5px] font-semibold text-ink-50">Email delivery</div>
            {loading ? (
              <Loader class="w-3.5 h-3.5 text-ink-300 animate-spin" />
            ) : configured ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="w-3.5 h-3.5" /> configured
              </span>
            ) : null}
          </div>
          <div class="text-[12px] text-ink-300 mt-1 leading-relaxed">
            Send mail from this server through an SMTP account.
          </div>
        </div>
      </header>

      <form onSubmit={save} class="p-3 space-y-3">
        <EmailProviderSelector providerId={form.providerId} onSelect={selectProvider} />
        <SMTPConnectionFields
          form={form}
          onFromAddress={setFromAddress}
          onHost={setHost}
          onPort={setPort}
          onTlsMode={setTlsMode}
          onAuthentication={setAuthentication}
          onUsername={setUsername}
          onPassword={setPassword}
        />
        {error && <div class="text-xs text-accent-red">{error}</div>}
        <div class="flex items-center gap-2">
          <button
            type="submit"
            disabled={saving || loading}
            class="btn btn-primary disabled:opacity-50 inline-flex items-center gap-2"
          >
            {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
            {configured ? "Update email settings" : "Save email settings"}
          </button>
          {configured && (
            <button type="button" onClick={remove} class="btn btn-secondary disabled:opacity-50">
              Remove
            </button>
          )}
        </div>
      </form>

      {configured && (
        <SMTPTestPanel
          defaultRecipient={defaultRecipient}
          testing={testing}
          testMessage={testMessage}
          onSendTest={sendTest}
        />
      )}
    </section>
  );
}
