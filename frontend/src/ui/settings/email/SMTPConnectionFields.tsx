import {
  AUTHENTICATION_MODE_OPTIONS,
  TLS_MODE_OPTIONS,
  type AuthenticationMode,
  type TLSMode,
} from "../../../config/constants/email-providers";
import type { SMTPFormInput } from "../../../model/email/application/smtpSettingsForm";
import { ExternalLink } from "../../primitives/icons";

const inputClass =
  "w-full h-10 rounded-md bg-inset border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue";

export function SMTPConnectionFields({
  form,
  onFromAddress,
  onHost,
  onPort,
  onTlsMode,
  onAuthentication,
  onUsername,
  onPassword,
}: {
  form: SMTPFormInput;
  onFromAddress: (value: string) => void;
  onHost: (value: string) => void;
  onPort: (value: string) => void;
  onTlsMode: (value: TLSMode) => void;
  onAuthentication: (value: AuthenticationMode) => void;
  onUsername: (value: string) => void;
  onPassword: (value: string) => void;
}) {
  const isGmail = form.providerId === "gmail";
  const authDisabled = form.authentication === "none";

  return (
    <div class="space-y-3">
      {isGmail && (
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
      )}

      <label class="block space-y-1.5">
        <span class="text-xs text-ink-300">Sender address</span>
        <input
          type="text"
          value={form.fromAddress}
          onInput={(e) => onFromAddress((e.currentTarget as HTMLInputElement).value)}
          autocomplete="off"
          spellcheck={false}
          class={inputClass}
        />
      </label>

      {!isGmail && (
        <div class="grid grid-cols-2 gap-3">
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Host</span>
            <input
              type="text"
              value={form.host}
              onInput={(e) => onHost((e.currentTarget as HTMLInputElement).value)}
              autocomplete="off"
              spellcheck={false}
              class={inputClass}
            />
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Port</span>
            <input
              type="text"
              inputMode="numeric"
              value={form.port}
              onInput={(e) => onPort((e.currentTarget as HTMLInputElement).value)}
              class={inputClass}
            />
          </label>
        </div>
      )}

      {!isGmail && (
        <div class="grid grid-cols-2 gap-3">
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Security</span>
            <select
              value={form.tlsMode}
              onChange={(e) => onTlsMode((e.currentTarget as HTMLSelectElement).value as TLSMode)}
              class={inputClass}
            >
              {TLS_MODE_OPTIONS.map((opt) => (
                <option value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">Authentication</span>
            <select
              value={form.authentication}
              disabled={form.tlsMode === "none"}
              onChange={(e) =>
                onAuthentication((e.currentTarget as HTMLSelectElement).value as AuthenticationMode)
              }
              class={`${inputClass} disabled:opacity-50`}
            >
              {AUTHENTICATION_MODE_OPTIONS.map((opt) => (
                <option value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </label>
        </div>
      )}

      {form.tlsMode === "none" && (
        <div class="rounded-md border border-line bg-tint p-2.5 text-[12px] text-ink-300 leading-relaxed">
          A plaintext connection only works with a trusted relay on a private network, and never
          carries a username or password.
        </div>
      )}

      {!authDisabled && (
        <>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">{isGmail ? "Gmail address" : "Username"}</span>
            <input
              type="text"
              value={form.username}
              onInput={(e) => onUsername((e.currentTarget as HTMLInputElement).value)}
              autocomplete="off"
              spellcheck={false}
              class={inputClass}
            />
          </label>
          <label class="block space-y-1.5">
            <span class="text-xs text-ink-300">
              {isGmail ? "App password" : "Password"}
              {form.passwordConfigured && (
                <span class="text-ink-400"> (leave blank to keep the current one)</span>
              )}
            </span>
            <input
              type="password"
              value={form.password}
              onInput={(e) => onPassword((e.currentTarget as HTMLInputElement).value)}
              placeholder={isGmail ? "xxxx xxxx xxxx xxxx" : undefined}
              autocomplete="new-password"
              class={`${inputClass} placeholder:text-ink-400`}
            />
          </label>
        </>
      )}
    </div>
  );
}
