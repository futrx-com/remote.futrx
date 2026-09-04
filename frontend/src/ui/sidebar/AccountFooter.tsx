import { LogOut, Settings } from "../primitives/icons";

export function AccountFooter({
  email,
  onOpenSettings,
  onSignOut,
}: {
  email: string;
  onOpenSettings?: () => void;
  onSignOut: () => void;
}) {
  function signOut(event: MouseEvent): void {
    event.preventDefault();
    onSignOut();
  }

  return (
    <footer class="safe-bottom-control flex items-center gap-2 border-t border-line px-2.5 pt-2.5 text-sm">
      <div class="grid h-7 w-7 flex-none place-items-center rounded-full bg-tint-strong text-[12px] font-semibold text-ink-200">
        {(email[0] || "?").toUpperCase()}
      </div>
      <span class="min-w-0 flex-1 truncate text-[12.5px] text-ink-300" title={email}>{email}</span>
      {onOpenSettings && (
        <button
          type="button"
          onClick={onOpenSettings}
          class="grid h-8 w-8 flex-none place-items-center rounded-control text-ink-400 transition-colors hover:bg-tint-strong hover:text-ink-50"
          title="Settings"
          aria-label="Settings"
        >
          <Settings class="w-4 h-4" />
        </button>
      )}
      <a
        href="/auth/logout"
        onClick={signOut}
        class="grid h-8 w-8 flex-none place-items-center rounded-control text-ink-400 transition-colors hover:bg-accent-red/10 hover:text-accent-red"
        title="Sign out"
        aria-label="Sign out"
      >
        <LogOut class="w-4 h-4" />
      </a>
    </footer>
  );
}
