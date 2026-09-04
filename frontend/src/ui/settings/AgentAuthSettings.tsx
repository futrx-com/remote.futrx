import { Fragment } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";
import type { AgentAuthProvider } from "../../models/auth";
import { agentAuthRegistryService } from "../../services/auth/agentAuthRegistryService.ts";
import { useAuthContext } from "../../state/context/AuthContext";
import { Check, ExternalLink, Key, Loader } from "../primitives/icons";

export function AgentAuthSettingsList({ gateOnly = false }: { gateOnly?: boolean }) {
  const { agentAuth } = useAuthContext();
  const providers = gateOnly
    ? agentAuth.providers.filter((entry) => entry.authentication.satisfiesAccessGate)
    : agentAuth.providers;

  if (agentAuth.loading && providers.length === 0) {
    return (
      <div class="flex items-center justify-center gap-2 py-8 text-sm text-ink-300">
        <Loader class="h-4 w-4 animate-spin" /> Loading agent authentication
      </div>
    );
  }

  if (agentAuth.error && providers.length === 0) {
    return (
      <div class="rounded border border-accent-red/25 bg-accent-red/[0.08] px-3 py-2 text-[12px] text-accent-red">
        {agentAuth.error}
      </div>
    );
  }

  return (
    <div class="space-y-3">
      {providers.map((entry) => (
        <AgentAuthSettings key={entry.provider} entry={entry} />
      ))}
    </div>
  );
}

function AgentAuthSettings({ entry }: { entry: AgentAuthProvider }) {
  const { agentAuth } = useAuthContext();
  const [code, setCode] = useState("");
  const [apiKey, setAPIKey] = useState("");
  const [apiKeyFormOpen, setAPIKeyFormOpen] = useState(false);
  const codeRef = useRef<HTMLTextAreaElement>(null);
  const loginActive = entry.status.login.active;
  const busy = !!agentAuth.starting[entry.provider];
  const managedCode = entry.authentication.mode === "managed-code";
  const managedDevice = entry.authentication.mode === "managed-device";
  const managedAPIKey = entry.authentication.mode === "managed-api-key";
  const managed = managedCode || managedDevice || managedAPIKey;
  const apiKeyCredentialLabel = entry.authentication.apiKey?.credentialLabel || `${entry.label} API key`;
  const loginInteractionActive = loginActive || (managedAPIKey && apiKeyFormOpen);
  const statusKind = agentAuthRegistryService.statusKind(entry);
  const error = agentAuth.actionErrors[entry.provider]
    || entry.status.login.error
    || entry.status.warning
    || "";
  const expiresAt = entry.status.login.expiresAt
    ? new Date(entry.status.login.expiresAt * 1000).toLocaleTimeString([], {
      hour: "numeric",
      minute: "2-digit",
    })
    : "";

  useEffect(() => {
    if (managedCode && loginActive && entry.status.login.awaitingCode) {
      setTimeout(() => codeRef.current?.focus(), 50);
    }
  }, [managedCode, loginActive, entry.status.login.awaitingCode]);

  function startLogin() {
    if (managedCode) {
      void agentAuth.startCodeLogin(entry.provider);
      return;
    }
    if (managedDevice) {
      void agentAuth.startDeviceLogin(entry.provider);
      return;
    }
    if (managedAPIKey) {
      setAPIKey("");
      setAPIKeyFormOpen(true);
    }
  }

  function submitCode() {
    const value = code.trim();
    if (!value) return;
    void agentAuth.submitCode(entry.provider, value);
  }

  function cancelCode() {
    setCode("");
    void agentAuth.cancelCodeLogin(entry.provider);
  }

  async function saveAPIKey() {
    const value = apiKey.trim();
    if (!value || busy) return;
    if (await agentAuth.saveAPIKey(entry.provider, value)) {
      setAPIKey("");
      setAPIKeyFormOpen(false);
    }
  }

  async function deleteAPIKey() {
    if (busy) return;
    if (await agentAuth.deleteAPIKey(entry.provider)) {
      setAPIKey("");
      setAPIKeyFormOpen(false);
    }
  }

  return (
    <section class="space-y-3 rounded-md border border-line bg-tint p-3">
      <div class="flex items-start gap-3">
        <div class="grid h-9 w-9 flex-none place-items-center rounded-md border border-line bg-tint text-ink-200">
          <Key class="h-4 w-4" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <div class="text-[14px] font-semibold text-ink-100">{entry.label} authentication</div>
            {statusKind === "no-auth" ? (
              <span class="text-[11px] text-accent-green">No sign-in required</span>
            ) : statusKind === "authenticated" ? (
              <span class="inline-flex items-center gap-1 text-[11px] text-accent-green">
                <Check class="h-3.5 w-3.5" /> Signed in
              </span>
            ) : statusKind === "external" ? (
              <span class="text-[11px] text-ink-400">Provider-managed</span>
            ) : (
              <span class="text-[11px] text-ink-400">not configured</span>
            )}
          </div>
          {entry.authentication.instructions && (
            <div class="mt-1 text-[12px] leading-relaxed text-ink-300">
              <InstructionText text={entry.authentication.instructions} />
            </div>
          )}
        </div>
      </div>

      {managed && (
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={startLogin}
            disabled={busy || loginInteractionActive}
            class="btn btn-primary disabled:opacity-50"
          >
            {busy
              ? managedAPIKey ? "Updating..." : "Starting..."
              : loginInteractionActive
                ? "Login in progress"
                : entry.status.authenticated
                  ? `Refresh ${entry.label} login`
                  : `Sign in with ${entry.label}`}
          </button>
          {agentAuth.loading && <Loader class="h-4 w-4 animate-spin text-ink-300" />}
        </div>
      )}

      {managedDevice && loginActive && (
        <div class="space-y-2 rounded border border-accent-blue/25 bg-accent-blue/[0.08] p-3">
          <div class="text-[12px] text-ink-200">Open the link and enter this code:</div>
          <div class="grid gap-2 sm:grid-cols-[1fr_auto]">
            <div class="rounded border border-line bg-inset px-3 py-2 font-mono text-[18px] tracking-wide text-ink-50">
              {entry.status.login.userCode || "Waiting for code..."}
            </div>
            {entry.status.login.url && (
              <a
                href={entry.status.login.url}
                target="_blank"
                rel="noreferrer"
                class="inline-flex h-10 items-center justify-center gap-2 rounded bg-tint-strong px-3 text-[13px] font-medium text-ink-100 hover:bg-tint-active"
              >
                <ExternalLink class="h-4 w-4" /> Open
              </a>
            )}
          </div>
          {expiresAt && <div class="text-[11px] text-ink-400">Code expires around {expiresAt}.</div>}
        </div>
      )}

      {managedAPIKey && apiKeyFormOpen && (
        <div class="space-y-3 rounded border border-accent-blue/25 bg-accent-blue/[0.08] p-3">
          <div class="text-[12px] leading-relaxed text-ink-200">
            Paste your {apiKeyCredentialLabel}. Remote stores it privately and never displays it again.
          </div>
          {entry.authentication.apiKey?.createUrl && (
            <a
              href={entry.authentication.apiKey.createUrl}
              target="_blank"
              rel="noreferrer"
              class="inline-flex items-center gap-1.5 text-[12px] font-medium text-accent-blue hover:underline"
            >
              <ExternalLink class="h-3.5 w-3.5" /> {entry.authentication.apiKey.createLabel}
            </a>
          )}
          <input
            type="password"
            value={apiKey}
            onInput={(event) => setAPIKey(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.isComposing) {
                event.preventDefault();
                void saveAPIKey();
              }
            }}
            placeholder={apiKeyCredentialLabel}
            aria-label={apiKeyCredentialLabel}
            autocomplete="off"
            autocapitalize="off"
            autocorrect="off"
            spellcheck={false}
            class="w-full rounded-md border border-line bg-inset px-3 py-2.5 font-mono text-[13px] text-ink-100 placeholder:text-ink-300 focus:border-accent-blue focus:outline-none"
          />
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => {
                setAPIKey("");
                setAPIKeyFormOpen(false);
              }}
              class="h-10 rounded px-3 text-[13px] text-ink-200 hover:bg-tint-strong hover:text-ink-100"
            >
              Cancel
            </button>
            {entry.status.authenticated && (
              <button
                type="button"
                onClick={() => void deleteAPIKey()}
                disabled={busy}
                class="h-10 rounded px-3 text-[13px] text-accent-red hover:bg-accent-red/[0.08] disabled:opacity-50"
              >
                Remove API key
              </button>
            )}
            <button
              type="button"
              onClick={() => void saveAPIKey()}
              disabled={!apiKey.trim() || busy}
              class="btn btn-primary ml-auto disabled:opacity-50"
            >
              {busy ? "Saving..." : entry.status.authenticated ? "Replace API key" : "Save API key"}
            </button>
          </div>
        </div>
      )}

      {managedCode && loginActive && (
        <div class="space-y-2 rounded border border-accent-blue/25 bg-accent-blue/[0.08] p-3">
          <div class="text-[12px] text-ink-200">
            Open the link, sign in, then paste the authorization code here:
          </div>
          {entry.status.login.url && (
            <a
              href={entry.status.login.url}
              target="_blank"
              rel="noreferrer"
              class="block break-all rounded border border-line bg-inset px-2.5 py-2 font-mono text-[12px] text-accent-blue hover:underline"
            >
              <ExternalLink class="mr-1 inline h-3.5 w-3.5 align-[-2px]" />
              {entry.status.login.url}
            </a>
          )}
          <textarea
            ref={codeRef}
            value={code}
            onInput={(event) => setCode(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
                event.preventDefault();
                submitCode();
              }
            }}
            placeholder="Paste your code here"
            rows={2}
            autocomplete="off"
            autocapitalize="off"
            autocorrect="off"
            spellcheck={false}
            class="w-full resize-none rounded-md border border-line bg-inset px-3 py-2.5 font-mono text-[13px] text-ink-100 placeholder:text-ink-300 focus:border-accent-blue focus:outline-none"
          />
          <div class="flex gap-2">
            <button
              type="button"
              onClick={cancelCode}
              class="h-10 rounded px-3 text-[13px] text-ink-200 hover:bg-tint-strong hover:text-ink-100"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={submitCode}
              disabled={!code.trim() || busy}
              class="btn btn-primary flex-1 disabled:opacity-50"
            >
              {busy ? "Finishing..." : "Submit code"}
            </button>
          </div>
        </div>
      )}

      {error && (
        <div class="break-words rounded border border-accent-red/25 bg-accent-red/[0.08] px-2.5 py-2 text-[12px] text-accent-red">
          {error}
        </div>
      )}
    </section>
  );
}

function InstructionText({ text }: { text: string }) {
  return (
    <>
      {text.split(/(`[^`]+`)/).map((part, index) =>
        part.startsWith("`") && part.endsWith("`") ? (
          <span key={index} class="font-mono text-ink-100">{part.slice(1, -1)}</span>
        ) : (
          <Fragment key={index}>{part}</Fragment>
        )
      )}
    </>
  );
}
