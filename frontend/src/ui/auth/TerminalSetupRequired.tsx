import { Lock, Terminal } from "../primitives/icons";

/**
 * Shown to remote visitors when the server admin credential has not been
 * initialised yet. The first-time setup can only be performed from the
 * server terminal, so we display a clear instruction rather than a form.
 */
export function TerminalSetupRequired() {
  return (
    <div class="app-shell grid place-items-center bg-app text-ink-100 p-5">
      <div class="w-full max-w-md space-y-6 text-center">
        <div class="mx-auto w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
          <Lock class="w-6 h-6 text-accent-blue" />
        </div>

        <div>
          <div class="text-lg font-semibold">Server setup required</div>
          <div class="text-xs text-ink-300 mt-1.5 leading-relaxed">
            This server has not been configured yet. For security, the initial
            administrator account must be created directly on the server.
          </div>
        </div>

        <div class="rounded-lg border border-line bg-surface p-4 text-left space-y-3">
          <div class="flex items-center gap-2 text-xs font-semibold text-ink-200 uppercase tracking-wide">
            <Terminal class="w-3.5 h-3.5 text-ink-400" />
            Run in the server terminal
          </div>
          <pre class="text-[12px] font-mono text-accent-blue bg-tint rounded-md px-3 py-2 overflow-x-auto whitespace-pre-wrap break-all leading-relaxed">
            remote setup
          </pre>
          <p class="text-[11.5px] text-ink-400 leading-relaxed">
            Follow the prompts to set the admin email and password. Once complete, refresh this page to sign in.
          </p>
        </div>

        <button
          type="button"
          onClick={() => window.location.reload()}
          class="btn btn-primary btn-sm text-[13px]"
        >
          Refresh after setup
        </button>
      </div>
    </div>
  );
}
