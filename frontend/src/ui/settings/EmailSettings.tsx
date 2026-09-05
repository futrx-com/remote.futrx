import { useState } from "preact/hooks";
import { useEmailSettingsController } from "../../state/hooks/server/useEmailSettingsController";
import { Check, ExternalLink, Loader, Mail } from "../primitives/icons";

export function EmailSettings({ defaultRecipient }: { defaultRecipient: string }) {
  const {
    address,
    appPassword,
    error,
    loading,
    remove,
    save,
    saving,
    sendTest,
    setAddress,
    setAppPassword,
    settings,
    testMessage,
    testing,
  } = useEmailSettingsController();

  const [testRecipient, setTestRecipient] = useState(defaultRecipient);

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
            ) : settings?.configured ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="w-3.5 h-3.5" /> configured
              </span>
            ) : null}
          </div>
          <div class="text-[12px] text-ink-300 mt-1 leading-relaxed">
            Send mail from this server through your Gmail account.
          </div>
        </div>
      </header>

      <form onSubmit={save} class="p-3 space-y-3">
        <div class="rounded-md border border-line bg-tint p-2.5 text-[12px] text-ink-300 leading-relaxed">
          Your Google account needs 2-Step Verification enabled to generate an app password.
          <a
            href="https://myaccount.google.com/apppasswords"
            target="_blank"
            rel="noreferrer"
            class="inline-flex items-center gap-1 mt-2 text-accent-blue hover:underline"
          >
            Create a Gmail app password <ExternalLink class="w-3.5 h-3.5" />
          </a>
        </div>

        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">Gmail address</span>
          <input
            type="text"
            value={address}
            onInput={(event) => setAddress((event.currentTarget as HTMLInputElement).value)}
            autocomplete="off"
            spellcheck={false}
            class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
          />
        </label>
        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">App password</span>
          <input
            type="password"
            value={appPassword}
            onInput={(event) => setAppPassword((event.currentTarget as HTMLInputElement).value)}
            placeholder="xxxx xxxx xxxx xxxx"
            autocomplete="new-password"
            class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 placeholder:text-ink-400 focus:outline-none focus:border-accent-blue"
          />
        </label>
        {error && <div class="text-xs text-accent-red">{error}</div>}
        <div class="flex items-center gap-2">
          <button
            type="submit"
            disabled={saving || loading}
            class="btn btn-primary disabled:opacity-50 inline-flex items-center gap-2"
          >
            {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
            {settings?.configured ? "Update email settings" : "Save email settings"}
          </button>
          {settings?.configured && (
            <button
              type="button"
              onClick={remove}
              class="btn btn-secondary disabled:opacity-50"
            >
              Remove
            </button>
          )}
        </div>
      </form>

      {settings?.configured && (
        <div class="px-3 pb-3 space-y-2 border-t border-line pt-3">
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Send a test email to</span>
            <input
              type="text"
              value={testRecipient}
              onInput={(event) => setTestRecipient((event.currentTarget as HTMLInputElement).value)}
              class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
            />
          </label>
          <button
            type="button"
            disabled={testing}
            onClick={() => sendTest(testRecipient)}
            class="btn btn-secondary disabled:opacity-50 inline-flex items-center gap-2"
          >
            {testing && <Loader class="w-3.5 h-3.5 animate-spin" />}
            Send test email
          </button>
          {testMessage && <div class="text-xs text-ink-300">{testMessage}</div>}
        </div>
      )}
    </section>
  );
}
