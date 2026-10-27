import { sidebarPreferenceService } from "../../services/workspace/sidebarPreferenceService.ts";
import { ChatSkeleton } from "../chat/ChatSkeleton";
import { ChevronLeft, ChevronRight, LogOut, Plus, Search, Settings } from "../primitives/icons";
import { Skeleton } from "../primitives/Skeleton";
import { SidebarSkeleton } from "../sidebar/SidebarSkeleton";
import { AppShell } from "./AppShell";

/**
 * The whole workspace in outline, for the stretch before WorkspaceProvider can
 * mount at all — the provider-auth check and the lazy route chunk. It cannot
 * read workspace state, so the sidebar chrome is drawn here rather than reused
 * from Sidebar; the collapse preference is read straight from storage so the
 * pane does not resize the moment the real sidebar takes over.
 *
 * Chrome that owes nothing to the network — the wordmark, the search field, the
 * header and footer controls — is drawn for real and dimmed. Only what is
 * genuinely waiting on data becomes a placeholder.
 */
export function WorkspaceSkeleton() {
  const collapsed = sidebarPreferenceService.readCollapsed();

  return (
    <AppShell
      sidebar={
        <aside
          data-collapsed={collapsed ? "true" : "false"}
          class={`codex-sidebar codex-window-frame safe-top hidden md:flex flex-col bg-surface
                  ${collapsed ? "md:w-[64px]" : "md:w-[300px]"}`}
        >
          <header class={`px-2.5 pt-2.5 pb-2 ${collapsed ? "md:px-2" : ""}`}>
            <div class={`flex items-center gap-1 min-h-9 ${collapsed ? "md:justify-center" : "mb-3"}`}>
              <div class={`flex flex-1 min-w-0 items-center gap-2 pl-1 ${collapsed ? "hidden" : ""}`}>
                <img
                  src="/icon-192.png"
                  alt=""
                  aria-hidden="true"
                  class="h-5 w-5 flex-none rounded object-cover"
                />
                <span class="truncate text-[13px] font-semibold tracking-[-0.01em] text-ink-50">
                  Remote workspace
                </span>
              </div>
              <span
                class={`grid h-8 w-8 flex-none place-items-center text-ink-500 ${collapsed ? "hidden" : ""}`}
                aria-hidden="true"
              >
                <Plus class="h-4 w-4" />
              </span>
              <span class="grid h-8 w-8 flex-none place-items-center text-ink-500" aria-hidden="true">
                {collapsed ? <ChevronRight class="h-4 w-4" /> : <ChevronLeft class="h-4 w-4" />}
              </span>
            </div>

            {!collapsed && (
              <div
                class="mt-2 flex h-8 items-center gap-2 rounded-control bg-tint px-2.5"
                aria-hidden="true"
              >
                <Search class="h-3.5 w-3.5 flex-none text-ink-500" />
                <span class="text-[13px] text-ink-500">Search</span>
              </div>
            )}
          </header>

          {!collapsed && (
            <>
              <div class="flex items-center justify-between gap-2 px-4 pb-1 pt-1.5">
                <span class="text-[10.5px] font-semibold uppercase tracking-[0.1em] text-ink-400">
                  Projects
                </span>
                <Skeleton class="h-2 w-7" />
              </div>

              <div class="flex-1 min-h-0 overflow-hidden px-2 pb-3">
                <SidebarSkeleton />
              </div>

              <footer class="safe-bottom-control flex items-center gap-2 border-t border-line px-2.5 pt-2.5">
                <Skeleton class="h-7 w-7 flex-none rounded-full" />
                <Skeleton class="h-2.5 w-[52%]" />
                <div class="flex-1" />
                <span class="grid h-8 w-8 flex-none place-items-center text-ink-500" aria-hidden="true">
                  <Settings class="h-4 w-4" />
                </span>
                <span class="grid h-8 w-8 flex-none place-items-center text-ink-500" aria-hidden="true">
                  <LogOut class="h-4 w-4" />
                </span>
              </footer>
            </>
          )}
        </aside>
      }
    >
      <ChatSkeleton />
    </AppShell>
  );
}
