import { useLocalAuthController } from "../../state/hooks/auth/useLocalAuthController";
import type { LoginMode } from "../../models/auth";
import { Key, Loader, MessageSquare } from "../primitives/icons";
import { TwoFactorChallengeStep } from "./TwoFactorChallengeStep";

export function LoginScreen({
  mode,
  adminEmail,
  localAdminConfigured,
  googleOAuthEnabled,
  onSuccess,
}: {
  mode: LoginMode;
  adminEmail: string;
  localAdminConfigured: boolean;
  googleOAuthEnabled: boolean;
  onSuccess: () => Promise<void>;
}) {
  const {
    confirmation,
    email,
    error,
    errorEmail,
    googleURL,
    oauthError,
    password,
    setConfirmation,
    setEmail,
    setPassword,
    setup,
    submit,
    submitting,
    challenge,
  } = useLocalAuthController({ mode, adminEmail, onSuccess });
  const title = mode === "claim"
    ? "Create your admin account"
    : mode === "legacy-setup"
      ? "Secure your admin account"
      : "Sign in";
  const description = mode === "claim"
    ? "This email and password will be the private administrator login for this server."
    : mode === "legacy-setup"
      ? "Create a local password so Google is no longer required for administrator access."
      : !localAdminConfigured
        ? "The existing administrator must sign in with Google once, then create a local password."
        : "Administrators use their local password. Invited users sign in with Google.";

  if (challenge.pending) {
    return (
      <div class="app-shell overflow-y-auto grid place-items-center bg-[#090b0f] text-ink-100 p-5">
        <TwoFactorChallengeStep
          code={challenge.code}
          error={challenge.error}
          submitting={challenge.submitting}
          onCodeChange={challenge.setCode}
          onSubmit={challenge.submit}
          onCancel={challenge.cancel}
        />
      </div>
    );
  }

  return (
    <div class="app-shell overflow-y-auto grid place-items-center bg-app text-ink-100 p-5">
      <div class="w-full max-w-sm space-y-5 py-6">
        <div class="flex flex-col items-center gap-3 text-center">
          <div class="w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
            {setup ? <Key class="w-6 h-6 text-accent-blue" /> : <MessageSquare class="w-7 h-7 text-accent-blue" />}
          </div>
          <div>
            <div class="text-xl font-semibold">{title}</div>
            <div class="text-xs text-ink-300 mt-1.5 leading-relaxed">{description}</div>
          </div>
        </div>

        {(setup || localAdminConfigured) && (
          <form onSubmit={submit} class="space-y-3">
            <label class="block space-y-1.5">
              <span class="text-xs text-ink-300">Admin email</span>
              <input
                type="email"
                value={email}
                readOnly={mode === "legacy-setup"}
                onInput={(event) => setEmail((event.currentTarget as HTMLInputElement).value)}
                autocomplete="username"
                class="w-full h-11 rounded-md bg-surface border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue read-only:opacity-70"
              />
            </label>
            <label class="block space-y-1.5">
              <span class="text-xs text-ink-300">Password</span>
              <input
                type="password"
                value={password}
                onInput={(event) => setPassword((event.currentTarget as HTMLInputElement).value)}
                autocomplete={setup ? "new-password" : "current-password"}
                minlength={setup ? 12 : undefined}
                class="w-full h-11 rounded-md bg-surface border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
              />
            </label>
            {setup && (
              <label class="block space-y-1.5">
                <span class="text-xs text-ink-300">Confirm password</span>
                <input
                  type="password"
                  value={confirmation}
                  onInput={(event) => setConfirmation((event.currentTarget as HTMLInputElement).value)}
                  autocomplete="new-password"
                  minlength={12}
                  class="w-full h-11 rounded-md bg-surface border border-line px-3 text-sm text-ink-100 focus:outline-none focus:border-accent-blue"
                />
              </label>
            )}
            <button
              type="submit"
              disabled={submitting}
              class="btn btn-primary btn-lg btn-block disabled:opacity-50 inline-flex items-center justify-center gap-2"
            >
              {submitting && <Loader class="w-4 h-4 animate-spin" />}
              {setup ? "Create admin account" : "Sign in as administrator"}
            </button>
          </form>
        )}

        {mode === "login" && googleOAuthEnabled && (
          <>
            {localAdminConfigured && (
              <div class="flex items-center gap-3 text-[11px] text-ink-400">
                <span class="h-px flex-1 bg-tint-strong" /> invited users <span class="h-px flex-1 bg-tint-strong" />
              </div>
            )}
            <a
              href={googleURL}
              class="inline-flex items-center justify-center gap-2.5 w-full bg-white hover:bg-ink-50 text-ink-800 font-medium text-sm rounded-md px-4 h-11"
            >
              <GoogleG class="w-4 h-4" /> Sign in with Google
            </a>
          </>
        )}

        {(error || oauthError) && (
          <div class="text-xs text-accent-red bg-accent-red/10 border border-accent-red/30 rounded-lg p-3 leading-relaxed">
            {error || (oauthError === "not-invited"
              ? `${errorEmail || "That account"} is not invited to this server.`
              : oauthError === "admin-password"
                ? "The administrator must sign in with the local password."
                : `Login error: ${oauthError}`)}
          </div>
        )}

        {mode === "claim" && (
          <div class="text-[11px] text-ink-400 text-center leading-relaxed">
            Your password is stored only as a salted Argon2id hash. It cannot be recovered or displayed.
          </div>
        )}
      </div>
    </div>
  );
}

function GoogleG(props: { class?: string }) {
  return (
    <svg viewBox="0 0 48 48" class={props.class}>
      <path fill="#FFC107" d="M43.6 20.5H42V20H24v8h11.3c-1.6 4.7-6.1 8-11.3 8-6.6 0-12-5.4-12-12s5.4-12 12-12c3.1 0 5.9 1.2 8 3l5.7-5.7C33.8 6.2 29.1 4 24 4 13 4 4 13 4 24s9 20 20 20 20-9 20-20c0-1.3-.1-2.4-.4-3.5z"/>
      <path fill="#FF3D00" d="M6.3 14.7l6.6 4.8C14.6 16 19 13 24 13c3.1 0 5.9 1.2 8 3l5.7-5.7C33.8 6.2 29.1 4 24 4 16.3 4 9.6 8.4 6.3 14.7z"/>
      <path fill="#4CAF50" d="M24 44c5 0 9.6-1.9 13.1-5l-6-5c-2 1.5-4.5 2.4-7.1 2.4-5.2 0-9.6-3.3-11.3-8l-6.5 5C9.5 39.6 16.2 44 24 44z"/>
      <path fill="#1976D2" d="M43.6 20.5H42V20H24v8h11.3c-.8 2.3-2.2 4.3-4.2 5.7l6 5c-.4.4 6.4-4.7 6.4-14.7 0-1.3-.1-2.4-.4-3.5z"/>
    </svg>
  );
}
