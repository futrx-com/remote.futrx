import { useState } from "preact/hooks";
import { useAuthContext } from "../../state/context/AuthContext";
import { Check, Key, Loader } from "../primitives/icons";

// Admin-only custom provider pill, matching KimiAuthSettings/ClaudeAuthSettings.
// Self-contained: live status comes from the shared AuthContext WS. Unlike the
// CLI-backed providers there is no OAuth handshake — the admin enters a display
// name, API key, and base URL, which are persisted on the host. The API key is
// never echoed back; only the saved name + base URL are shown when connected.
export function CustomAuthSettings() {
  const { customAuth } = useAuthContext();
  const [editing, setEditing] = useState(!customAuth.authenticated);
  const [name, setName] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");

  const authenticated = customAuth.authenticated;
  const loading = customAuth.loading;
  const saving = customAuth.saving;
  const errorMessage = customAuth.error;

  async function submit() {
    if (!name.trim() || !apiKey.trim() || !baseUrl.trim()) return;
    try {
      await customAuth.save(name.trim(), apiKey.trim(), baseUrl.trim());
      setApiKey("");
      setEditing(false);
    } catch {
      // error surfaced via customAuth.error
    }
  }

  return (
    <section class="rounded-md border border-white/10 bg-white/[0.03] p-3 space-y-3">
      <div class="flex items-start gap-3">
        <div class="h-9 w-9 rounded-md bg-white/[0.06] border border-white/10 grid place-items-center flex-none text-ink-200">
          <Key class="w-4 h-4" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="text-[14px] font-semibold text-ink-100">Custom provider</div>
            {loading ? (
              <Loader class="w-3.5 h-3.5 text-ink-300 animate-spin" />
            ) : authenticated ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="w-3.5 h-3.5" /> Connected
              </span>
            ) : (
              <span class="text-[11px] text-ink-400">not configured</span>
            )}
          </div>
          <div class="text-[12px] text-ink-300 mt-1 leading-relaxed">
            Bring your own AI provider by entering a display name, an API key, and a base URL. The key is
            stored on the host and never shown again after saving.
          </div>
        </div>
      </div>

      {authenticated && !editing && customAuth.config && (
        <div class="rounded border border-white/10 bg-black/20 p-3 space-y-1">
          <div class="text-[12px] text-ink-300">Connected provider</div>
          <div class="text-[13px] text-ink-100 font-medium">{customAuth.config.name}</div>
          <div class="font-mono text-[12px] text-ink-200 break-all">{customAuth.config.baseUrl}</div>
        </div>
      )}

      {(!authenticated || editing) && (
        <div class="space-y-2">
          <label class="block space-y-1">
            <span class="text-[12px] text-ink-200">Display name</span>
            <input
              type="text"
              value={name}
              onInput={(e) => setName((e.currentTarget as HTMLInputElement).value)}
              placeholder="My provider"
              autocomplete="off"
              spellcheck={false}
              class="w-full rounded-md bg-black/30 border border-white/10 text-ink-100 placeholder:text-ink-300 px-3 py-2 text-[13px] focus:outline-none focus:border-accent-blue"
            />
          </label>
          <label class="block space-y-1">
            <span class="text-[12px] text-ink-200">API key</span>
            <input
              type="password"
              value={apiKey}
              onInput={(e) => setApiKey((e.currentTarget as HTMLInputElement).value)}
              placeholder="sk-..."
              autocomplete="off"
              autocapitalize="off"
              spellcheck={false}
              class="w-full rounded-md bg-black/30 border border-white/10 text-ink-100 placeholder:text-ink-300 px-3 py-2 font-mono text-[13px] focus:outline-none focus:border-accent-blue"
            />
          </label>
          <label class="block space-y-1">
            <span class="text-[12px] text-ink-200">Base URL</span>
            <input
              type="text"
              value={baseUrl}
              onInput={(e) => setBaseUrl((e.currentTarget as HTMLInputElement).value)}
              placeholder="https://api.example.com/v1"
              autocomplete="off"
              autocapitalize="off"
              spellcheck={false}
              class="w-full rounded-md bg-black/30 border border-white/10 text-ink-100 placeholder:text-ink-300 px-3 py-2 font-mono text-[13px] focus:outline-none focus:border-accent-blue"
            />
          </label>
          <div class="flex items-center gap-2">
            <button
              type="button"
              onClick={() => void submit()}
              disabled={saving || !name.trim() || !apiKey.trim() || !baseUrl.trim()}
              class="h-10 px-3 rounded bg-accent-blue/80 hover:bg-accent-blue text-white text-[13px] font-medium disabled:opacity-50"
            >
              {saving ? "Saving..." : "Save"}
            </button>
            {authenticated && editing && (
              <button
                type="button"
                onClick={() => {
                  setEditing(false);
                  setApiKey("");
                }}
                class="h-10 px-3 text-[13px] text-ink-200 hover:text-ink-100 hover:bg-white/[0.08] rounded"
              >
                Cancel
              </button>
            )}
            {saving && <Loader class="w-4 h-4 text-ink-300 animate-spin" />}
          </div>
        </div>
      )}

      {authenticated && !editing && (
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => {
              setEditing(true);
              setName(customAuth.config?.name ?? "");
              setBaseUrl(customAuth.config?.baseUrl ?? "");
            }}
            class="h-10 px-3 rounded bg-white/[0.08] hover:bg-white/[0.12] text-ink-100 text-[13px] font-medium"
          >
            Reconfigure
          </button>
        </div>
      )}

      {errorMessage && (!authenticated || editing) && (
        <div class="text-[12px] text-accent-red bg-accent-red/[0.08] border border-accent-red/25 rounded px-2.5 py-2 break-words">
          {errorMessage}
        </div>
      )}
    </section>
  );
}
