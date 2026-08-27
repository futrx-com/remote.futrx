import type { PushBlocker } from "../../models/push";
import type { PushNotifications } from "../../state/hooks/push/usePushNotifications";
import { Bell, BellOff, Check, Loader, Smartphone } from "../primitives/icons";

// Each blocker gets copy the user can act on, or an honest explanation that
// they cannot.
const blockerCopy: Record<PushBlocker, { title: string; detail: string }> = {
  unsupported: {
    title: "This browser cannot receive notifications",
    detail: "It does not support service workers or the Push API. Try a recent Chrome, Edge, Firefox, or Safari.",
  },
  "install-required": {
    title: "Add remote to your Home Screen first",
    detail:
      "On iPhone and iPad, notifications only work for installed web apps. Tap Share, then “Add to Home Screen”, and open remote from there.",
  },
  denied: {
    title: "Notifications are blocked",
    detail:
      "This site's notification permission was denied. Re-allow it in your browser's site settings, then reload this page.",
  },
  "server-disabled": {
    title: "Push notifications are not configured on this server",
    detail:
      "The server could not create its Web Push signing key. Check the backend logs for a line starting with “push:”.",
  },
};

export function NotificationSettings({ push }: { push: PushNotifications }) {
  const enabled = push.status === "on";

  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-white/[0.06]">
        <div class="mt-0.5 w-9 h-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none">
          {enabled ? (
            <Bell class="w-4 h-4 text-accent-blue" />
          ) : (
            <BellOff class="w-4 h-4 text-ink-200" />
          )}
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Notifications</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
            Get notified on this device when an agent needs you
          </div>
        </div>
        {(push.status === "loading" || push.busy) && (
          <Loader class="w-4 h-4 mt-2 text-ink-300 animate-spin" />
        )}
      </header>

      <div class="p-4 space-y-4">
        {push.status === "blocked" && push.blocker ? (
          <div class="rounded-lg border border-white/10 bg-white/[0.03] p-3 flex items-start gap-3">
            <Smartphone class="w-4 h-4 mt-0.5 flex-none text-ink-300" />
            <div class="min-w-0">
              <div class="text-[13px] text-ink-100">{blockerCopy[push.blocker].title}</div>
              <p class="text-[12px] text-ink-300 mt-1 leading-relaxed">
                {blockerCopy[push.blocker].detail}
              </p>
            </div>
          </div>
        ) : (
          <>
            <ul class="space-y-1.5 text-[12.5px] text-ink-300">
              <li class="flex items-start gap-2">
                <span class="mt-1.5 w-1 h-1 rounded-full bg-accent-blue flex-none" />
                The agent asks a question and is waiting on your answer
              </li>
              <li class="flex items-start gap-2">
                <span class="mt-1.5 w-1 h-1 rounded-full bg-ink-400 flex-none" />
                A turn finishes, or a run fails
              </li>
              <li class="flex items-start gap-2">
                <span class="mt-1.5 w-1 h-1 rounded-full bg-ink-400 flex-none" />
                A scheduled task finishes while you are away
              </li>
            </ul>

            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                disabled={push.busy || push.status === "loading"}
                onClick={() => void (enabled ? push.disable() : push.enable())}
                class={`h-9 px-3.5 rounded-md text-[13px] font-medium transition inline-flex items-center gap-2
                        disabled:opacity-60 disabled:cursor-wait ${
                          enabled
                            ? "border border-white/10 text-ink-100 hover:bg-white/[0.07]"
                            : "bg-accent-blue text-white hover:brightness-110"
                        }`}
              >
                {enabled ? <BellOff class="w-3.5 h-3.5" /> : <Bell class="w-3.5 h-3.5" />}
                {enabled ? "Turn off on this device" : "Turn on for this device"}
              </button>

              {enabled && (
                <button
                  type="button"
                  disabled={push.testing}
                  onClick={() => void push.sendTest()}
                  class="h-9 px-3.5 rounded-md text-[13px] border border-white/10 text-ink-200
                         hover:text-ink-50 hover:bg-white/[0.07] transition
                         disabled:opacity-60 disabled:cursor-wait"
                >
                  {push.testing ? "Sending" : "Send a test"}
                </button>
              )}
            </div>

            <p class="text-[11.5px] text-ink-400 leading-relaxed">
              Notifications are per device — turn them on once on each phone or computer you want
              alerted. Nothing is sent while you are already looking at that chat.
            </p>
          </>
        )}

        <div class="min-h-5 text-[12px]">
          {push.error ? (
            <span class="text-accent-red">{push.error}</span>
          ) : push.notice ? (
            <span class="inline-flex items-center gap-1 text-accent-green">
              <Check class="w-3.5 h-3.5" /> {push.notice}
            </span>
          ) : enabled ? (
            <span class="inline-flex items-center gap-1 text-accent-green">
              <Check class="w-3.5 h-3.5" /> On for this device
            </span>
          ) : null}
        </div>
      </div>
    </section>
  );
}
