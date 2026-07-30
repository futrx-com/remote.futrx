import { useAuthContext } from "../../state/context/AuthContext";
import { ClaudeAuthSettings } from "../settings/ClaudeAuthSettings";
import { CodexAuthSettings } from "../settings/CodexAuthSettings";
import { CustomAuthSettings } from "../settings/CustomAuthSettings";
import { KimiAuthSettings } from "../settings/KimiAuthSettings";
import { Key } from "../primitives/icons";

export function ProviderLoginScreen() {
  const { codexAuth, kimiAuth } = useAuthContext();

  return (
    <div class="app-shell overflow-y-auto bg-[#090b0f] text-ink-100 p-5">
      <div class="w-full max-w-2xl mx-auto py-6 space-y-5">
        <div class="flex flex-col items-center gap-3 text-center">
          <div class="w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
            <Key class="w-6 h-6 text-accent-blue" />
          </div>
          <div>
            <div class="text-xl font-semibold">Connect an AI provider</div>
            <div class="text-sm text-ink-300 mt-1.5 leading-relaxed">
              Sign in to at least one provider to continue. You can connect the others later.
            </div>
          </div>
        </div>

        <div class="space-y-3">
          <ClaudeAuthSettings />
          <CodexAuthSettings
            authenticated={codexAuth.authenticated}
            usesApiKey={codexAuth.usesApiKey}
            deviceLogin={codexAuth.deviceLogin}
            loading={codexAuth.loading}
            starting={codexAuth.starting}
            error={codexAuth.error}
            onStartDeviceLogin={codexAuth.startDeviceLogin}
          />
          <KimiAuthSettings
            authenticated={kimiAuth.authenticated}
            deviceLogin={kimiAuth.deviceLogin}
            loading={kimiAuth.loading}
            starting={kimiAuth.starting}
            error={kimiAuth.error}
            onStartDeviceLogin={kimiAuth.startDeviceLogin}
          />
          <CustomAuthSettings />
        </div>
      </div>
    </div>
  );
}
