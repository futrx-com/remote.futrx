import { Key, Loader } from "../primitives/icons";

export function AdminSetupWaiting({ adminEmail }: { adminEmail?: string }) {
  return (
    <div class="app-shell grid place-items-center bg-app text-ink-100 p-5">
      <div class="w-full max-w-md space-y-5 text-center">
        <div class="mx-auto w-14 h-14 rounded-lg bg-accent-blue/[0.14] border border-accent-blue/25 grid place-items-center">
          <Key class="w-6 h-6 text-accent-blue" />
        </div>
        <div>
          <div class="text-lg font-semibold">Administrator setup required</div>
          <div class="text-xs text-ink-300 mt-1.5 leading-relaxed">
            The administrator{adminEmail ? ` (${adminEmail})` : ""} must create a local password before the workspace opens.
          </div>
        </div>
        <div class="flex items-center justify-center gap-2 text-sm text-ink-300">
          <Loader class="w-4 h-4 animate-spin" /> Waiting for the administrator…
        </div>
      </div>
    </div>
  );
}
