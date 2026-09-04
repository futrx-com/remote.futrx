import { useGoogleOAuthSettingsController } from "../../state/hooks/auth/useGoogleOAuthSettingsController";
import { Check, ExternalLink, Key, Loader } from "../primitives/icons";

export function GoogleOAuthSettings() {
  const {
    clientId,
    clientSecret,
    error,
    loading,
    save,
    saving,
    setClientId,
    setClientSecret,
    settings,
  } = useGoogleOAuthSettingsController();

  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
          <Key class="w-4 h-4 text-ink-200" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="text-[14.5px] font-semibold text-ink-50">Google sign-in for users</div>
            {loading ? (
              <Loader class="w-3.5 h-3.5 text-ink-300 animate-spin" />
            ) : settings?.configured ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="w-3.5 h-3.5" /> configured
              </span>
            ) : (
              <span class="text-[11px] text-accent-yellow">required before inviting users</span>
            )}
          </div>
          <div class="text-[12px] text-ink-300 mt-1 leading-relaxed">
            Administrators use the local password. Invited users use these Google OAuth credentials.
          </div>
        </div>
      </header>

      <form onSubmit={save} class="p-3 space-y-3">
        <div class="rounded-md border border-line bg-tint p-2.5 text-[12px] text-ink-300 leading-relaxed">
          Create an OAuth 2.0 <span class="text-ink-100">Web application</span> in Google Cloud and add this authorized redirect URI:
          <code class="block mt-2 break-all rounded bg-inset border border-line px-2.5 py-2 text-[11.5px] text-ink-100">
            {settings?.redirectUrl || `${location.origin}/auth/google/callback`}
          </code>
          <a
            href="https://console.cloud.google.com/apis/credentials"
            target="_blank"
            rel="noreferrer"
            class="inline-flex items-center gap-1 mt-2 text-accent-blue hover:underline"
          >
            Open Google Cloud credentials <ExternalLink class="w-3.5 h-3.5" />
          </a>
        </div>

        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">Google client ID</span>
          <input
            type="text"
            value={clientId}
            onInput={(event) => setClientId((event.currentTarget as HTMLInputElement).value)}
            autocomplete="off"
            spellcheck={false}
            class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
          />
        </label>
        <label class="block space-y-1.5">
          <span class="text-xs text-ink-300">Google client secret</span>
          <input
            type="password"
            value={clientSecret}
            onInput={(event) => setClientSecret((event.currentTarget as HTMLInputElement).value)}
            placeholder={settings?.configured ? "Enter a new secret to replace the current one" : "Client secret"}
            autocomplete="new-password"
            class="w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 placeholder:text-ink-400 focus:outline-none focus:border-accent-blue"
          />
        </label>
        {error && <div class="text-xs text-accent-red">{error}</div>}
        <button
          type="submit"
          disabled={saving || loading}
          class="btn btn-primary disabled:opacity-50 inline-flex items-center gap-2"
        >
          {saving && <Loader class="w-3.5 h-3.5 animate-spin" />}
          {settings?.configured ? "Update Google sign-in" : "Save Google sign-in"}
        </button>
      </form>
    </section>
  );
}
