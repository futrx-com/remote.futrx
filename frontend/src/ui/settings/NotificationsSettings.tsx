import type { NotificationEventToggles } from "../../models/notifications";
import { useNotificationsSettingsController } from "../../state/hooks/notifications/useNotificationsSettingsController";
import { Bell, Check, ExternalLink, Loader, Send, X } from "../primitives/icons";

const EVENT_ROWS: Array<{ key: keyof NotificationEventToggles; label: string; hint: string }> = [
  { key: "runFinished", label: "Run finished", hint: "An agent turn completed successfully." },
  { key: "runFailed", label: "Run failed", hint: "An agent turn errored or was cancelled." },
  {
    key: "needsAttention",
    label: "Needs attention",
    hint: "The agent asked a question or is waiting for a plan approval.",
  },
  { key: "scheduledRun", label: "Scheduled task", hint: "A scheduled task finished or failed." },
];

export function NotificationsSettings() {
  const {
    botToken,
    chatId,
    clearTelegramToken,
    clearWebhookSecret,
    error,
    events,
    loading,
    notificationsEnabled,
    save,
    saved,
    saving,
    sendTest,
    setBotToken,
    setChatId,
    setNotificationsEnabled,
    setWebhookSecret,
    setWebhookUrl,
    settings,
    testing,
    testResults,
    toggleEvent,
    webhookSecret,
    webhookUrl,
  } = useNotificationsSettingsController();

  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
        <div class="h-9 w-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none">
          <Bell class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="text-[14.5px] font-semibold text-ink-50">Outbound notifications</div>
            {loading ? (
              <Loader class="w-3.5 h-3.5 text-ink-300 animate-spin" />
            ) : settings?.enabled ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="w-3.5 h-3.5" /> on
              </span>
            ) : (
              <span class="text-[11px] text-ink-400">off</span>
            )}
          </div>
          <div class="text-[12px] text-ink-300 mt-1 leading-relaxed">
            Get pinged on your phone when an agent run finishes, fails, or needs you — and when a
            scheduled task runs. Every message links straight back to the chat.
          </div>
        </div>
      </header>

      <form onSubmit={save} class="p-3 space-y-4">
        <label class="flex items-start gap-2.5 rounded-md border border-white/10 bg-white/[0.03] p-2.5 cursor-pointer">
          <input
            type="checkbox"
            checked={notificationsEnabled}
            onChange={(event) =>
              setNotificationsEnabled((event.currentTarget as HTMLInputElement).checked)
            }
            class="mt-0.5 h-4 w-4 flex-none accent-accent-blue"
          />
          <span class="min-w-0">
            <span class="block text-[13px] text-ink-100">Send notifications</span>
            <span class="block text-[12px] text-ink-300 leading-relaxed">
              Configure at least one destination below before turning this on.
            </span>
          </span>
        </label>

        <fieldset class="space-y-3">
          <legend class="text-xs font-semibold text-ink-200 uppercase tracking-wide">Telegram</legend>
          <div class="rounded-md border border-white/10 bg-white/[0.03] p-2.5 text-[12px] text-ink-300 leading-relaxed">
            Message <span class="text-ink-100">@BotFather</span> on Telegram, send{" "}
            <code class="rounded bg-black/30 border border-white/10 px-1 py-0.5 text-[11.5px]">
              /newbot
            </code>
            , and paste the token it gives you. Then send your bot a message and read your chat ID
            from{" "}
            <code class="rounded bg-black/30 border border-white/10 px-1 py-0.5 text-[11.5px] break-all">
              https://api.telegram.org/bot&lt;token&gt;/getUpdates
            </code>
            .
            <a
              href="https://core.telegram.org/bots/features#botfather"
              target="_blank"
              rel="noreferrer"
              class="inline-flex items-center gap-1 mt-2 text-accent-blue hover:underline"
            >
              Telegram bot documentation <ExternalLink class="w-3.5 h-3.5" />
            </a>
          </div>

          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Bot token</span>
            <input
              type="password"
              value={botToken}
              onInput={(event) => setBotToken((event.currentTarget as HTMLInputElement).value)}
              placeholder={
                settings?.telegram.configured
                  ? `Stored (${settings.telegram.botTokenMasked}) — enter a new token to replace it`
                  : "123456789:AA..."
              }
              autocomplete="new-password"
              spellcheck={false}
              class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 placeholder:text-ink-400 focus:outline-none focus:border-accent-blue"
            />
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Chat ID</span>
            <input
              type="text"
              value={chatId}
              onInput={(event) => setChatId((event.currentTarget as HTMLInputElement).value)}
              placeholder="-1001234567890"
              autocomplete="off"
              spellcheck={false}
              class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 placeholder:text-ink-400 focus:outline-none focus:border-accent-blue"
            />
          </label>
          {settings?.telegram.configured && (
            <button
              type="button"
              onClick={() => void clearTelegramToken()}
              disabled={saving}
              class="h-8 px-2.5 rounded-md border border-white/10 text-ink-300 hover:text-ink-100 hover:bg-white/[0.06] text-[12px] disabled:opacity-50"
            >
              Remove stored bot token
            </button>
          )}
        </fieldset>

        <fieldset class="space-y-3">
          <legend class="text-xs font-semibold text-ink-200 uppercase tracking-wide">Webhook</legend>
          <div class="rounded-md border border-white/10 bg-white/[0.03] p-2.5 text-[12px] text-ink-300 leading-relaxed">
            Remote POSTs a JSON body to this URL. With a shared secret set, each request carries{" "}
            <code class="rounded bg-black/30 border border-white/10 px-1 py-0.5 text-[11.5px] break-all">
              X-Remote-Signature: sha256=&lt;hmac&gt;
            </code>{" "}
            over the exact body.
          </div>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Webhook URL</span>
            <input
              type="url"
              value={webhookUrl}
              onInput={(event) => setWebhookUrl((event.currentTarget as HTMLInputElement).value)}
              placeholder="https://hooks.example.com/remote"
              autocomplete="off"
              spellcheck={false}
              class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 placeholder:text-ink-400 focus:outline-none focus:border-accent-blue"
            />
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Shared secret (optional)</span>
            <input
              type="password"
              value={webhookSecret}
              onInput={(event) => setWebhookSecret((event.currentTarget as HTMLInputElement).value)}
              placeholder={
                settings?.webhook.secretMasked
                  ? `Stored (${settings.webhook.secretMasked}) — enter a new secret to replace it`
                  : "Used to sign each request"
              }
              autocomplete="new-password"
              class="w-full h-10 rounded-md bg-black/30 border border-white/10 px-3 text-sm text-ink-100 placeholder:text-ink-400 focus:outline-none focus:border-accent-blue"
            />
          </label>
          {settings?.webhook.secretMasked && (
            <button
              type="button"
              onClick={() => void clearWebhookSecret()}
              disabled={saving}
              class="h-8 px-2.5 rounded-md border border-white/10 text-ink-300 hover:text-ink-100 hover:bg-white/[0.06] text-[12px] disabled:opacity-50"
            >
              Remove stored webhook secret
            </button>
          )}
        </fieldset>

        <fieldset class="space-y-2">
          <legend class="text-xs font-semibold text-ink-200 uppercase tracking-wide">Events</legend>
          {EVENT_ROWS.map((row) => (
            <label
              key={row.key}
              class="flex items-start gap-2.5 rounded-md border border-white/10 bg-white/[0.03] p-2.5 cursor-pointer"
            >
              <input
                type="checkbox"
                checked={events[row.key]}
                onChange={(event) =>
                  toggleEvent(row.key, (event.currentTarget as HTMLInputElement).checked)
                }
                class="mt-0.5 h-4 w-4 flex-none accent-accent-blue"
              />
              <span class="min-w-0">
                <span class="block text-[13px] text-ink-100">{row.label}</span>
                <span class="block text-[12px] text-ink-300 leading-relaxed">{row.hint}</span>
              </span>
            </label>
          ))}
        </fieldset>

        {error && <div class="text-xs text-accent-red">{error}</div>}
        {saved && !error && <div class="text-xs text-accent-green">Notification settings saved.</div>}

        <div class="flex flex-wrap items-center gap-2">
          <button
            type="submit"
            disabled={saving || loading}
            class="h-10 px-3 rounded-md bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50 inline-flex items-center gap-2"
          >
            {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
            Save notifications
          </button>
          <button
            type="button"
            onClick={() => void sendTest()}
            disabled={testing || saving || loading}
            class="h-10 px-3 rounded-md border border-white/10 text-ink-100 hover:bg-white/[0.06] text-[13px] font-medium disabled:opacity-50 inline-flex items-center gap-2"
          >
            {testing ? <Loader class="w-3.5 h-3.5 animate-spin" /> : <Send class="w-3.5 h-3.5" />}
            Send test
          </button>
        </div>

        {testResults && (
          <ul class="space-y-1.5">
            {testResults.map((result) => (
              <li
                key={result.sink}
                class="flex items-start gap-2 rounded-md border border-white/10 bg-white/[0.03] p-2.5 text-[12px]"
              >
                {result.delivered ? (
                  <Check class="w-3.5 h-3.5 mt-0.5 flex-none text-accent-green" />
                ) : (
                  <X class="w-3.5 h-3.5 mt-0.5 flex-none text-accent-red" />
                )}
                <span class="min-w-0">
                  <span class="text-ink-100">{result.sink}</span>
                  <span class="text-ink-300">
                    {result.delivered ? " delivered the test message." : ` failed: ${result.error}`}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </form>
    </section>
  );
}
