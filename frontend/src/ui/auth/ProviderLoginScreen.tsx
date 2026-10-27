import { AgentAuthSettingsList } from "../settings/AgentAuthSettings";
import { Key } from "../primitives/icons";

export function ProviderLoginScreen() {
  return (
    <div class="app-shell overflow-y-auto bg-app p-5 text-ink-100">
      <div class="mx-auto w-full max-w-2xl space-y-5 py-6">
        <div class="flex flex-col items-center gap-3 text-center">
          <div class="grid h-14 w-14 place-items-center rounded-lg border border-accent-blue/25 bg-accent-blue/[0.14]">
            <Key class="h-6 w-6 text-accent-blue" />
          </div>
          <div>
            <div class="text-xl font-semibold">Connect an AI provider</div>
            <div class="mt-1.5 text-sm leading-relaxed text-ink-300">
              Sign in to at least one provider to continue. You can connect the others later.
            </div>
          </div>
        </div>

        <AgentAuthSettingsList gateOnly />
      </div>
    </div>
  );
}
