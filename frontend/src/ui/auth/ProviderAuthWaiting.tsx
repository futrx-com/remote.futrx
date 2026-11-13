import { Key, Loader } from "../primitives/icons";

export function ProviderAuthWaiting({ adminEmail }: { adminEmail?: string }) {
  return (
    <div class="app-shell grid place-items-center bg-app text-ink-100 p-5">
      <div class="w-full max-w-md space-y-6 text-center">
        <div class="flex flex-col items-center gap-3">
          <div class="w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
            <Key class="w-6 h-6 text-accent-blue" />
          </div>
          <div>
            <div class="text-lg font-semibold">You don't have access to an AI provider yet</div>
            <div class="text-xs text-ink-300 mt-1.5 leading-relaxed">
              Ask an admin
              {adminEmail ? (
                <> (<span class="font-mono text-ink-100">{adminEmail}</span>)</>
              ) : null}{" "}
              to give you access to an AI provider account. This page continues
              automatically once you have access.
            </div>
          </div>
        </div>
        <div class="flex items-center justify-center gap-2 text-ink-300 text-sm">
          <Loader class="w-4 h-4 animate-spin" /> Waiting for access…
        </div>
      </div>
    </div>
  );
}
