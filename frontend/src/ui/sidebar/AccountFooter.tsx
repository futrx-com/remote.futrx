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
    <footer class="safe-bottom-control border-t border-white/10 px-3 pt-3 flex items-center gap-2 text-sm bg-[#0d1015]">
      <div class="w-9 h-9 rounded-md bg-accent-green/15 text-accent-green grid place-items-center font-semibold flex-none">
        {(email[0] || "?").toUpperCase()}
      </div>
      <span class="flex-1 min-w-0 truncate text-ink-200" title={email}>{email}</span>
      {onOpenSettings && (
        <button
          type="button"
          onClick={onOpenSettings}
          class="h-9 w-9 rounded-md text-ink-300 hover:text-ink-50 hover:bg-white/[0.08] grid place-items-center flex-none"
          title="Settings"
          aria-label="Settings"
        >
          <Settings class="w-4 h-4" />
        </button>
      )}
      <a
        href="/auth/logout"
        onClick={signOut}
        class="h-9 w-9 rounded-md text-ink-300 hover:text-accent-red hover:bg-accent-red/10 grid place-items-center flex-none"
        title="Sign out"
        aria-label="Sign out"
      >
        <LogOut class="w-4 h-4" />
      </a>
    </footer>
  );
}
