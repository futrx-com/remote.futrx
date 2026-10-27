import {
  ArrowUp,
  CalendarClock,
  ChevronRight,
  Code,
  Folder,
  Menu,
  Monitor,
  Plus,
  Terminal,
} from "../primitives/icons";
import { Skeleton } from "../primitives/Skeleton";
import { MessageSkeleton } from "./messages/MessageSkeleton";

// Chrome is drawn for real and dimmed; only the parts that are waiting on data
// become placeholders. Grey blobs where the toolbar and send button belong read
// as a broken page, and they move when the real icons arrive.
const HEADER_ACTIONS = [Code, Terminal, Folder, CalendarClock, Monitor];

/**
 * Stand-in for the whole main pane while the workspace is still resolving which
 * chat to show. It traces ChatThread's own frame — header, measured message
 * column, composer card — so the real thread swaps in without the layout
 * moving, and never shows an empty-state pitch the data may contradict.
 *
 * The hamburger stays live where there is a sidebar to open; during boot there
 * is nothing behind it yet, so it renders as inert chrome.
 */
export function ChatSkeleton({ onHamburger }: { onHamburger?: () => void }) {
  return (
    <div class="codex-thread flex-1 h-full flex min-h-0 overflow-hidden bg-canvas">
      <div class="flex min-w-0 flex-1 flex-col">
        <header class="codex-header top-chrome z-20 flex flex-none items-center gap-2 border-b border-line px-5 pb-2">
          {onHamburger ? (
            <button
              type="button"
              onClick={onHamburger}
              class="grid h-8 w-8 flex-none place-items-center rounded-control text-ink-300
                     transition-colors hover:bg-tint-strong hover:text-ink-50 md:hidden"
              aria-label="Open chats"
              title="Chats"
            >
              <Menu class="h-4 w-4" />
            </button>
          ) : (
            <div class="h-8 w-8 flex-none md:hidden" aria-hidden="true" />
          )}

          {/* The breadcrumb keeps its real spine — project, chevron, title,
              status dot — with only the two names left to arrive. */}
          <div class="flex min-w-0 flex-1 items-center gap-1.5">
            <Skeleton class="hidden h-2.5 w-14 sm:block" />
            <ChevronRight class="hidden h-3 w-3 flex-none text-ink-500 sm:block" aria-hidden="true" />
            <Skeleton class="h-2.5 w-28 max-w-[45%]" />
            <span class="ml-0.5 h-1.5 w-1.5 flex-none rounded-full bg-ink-500" aria-hidden="true" />
          </div>

          <div class="hidden flex-none items-center gap-0.5 md:flex" aria-hidden="true">
            {HEADER_ACTIONS.map((Icon, index) => (
              <span key={index} class="grid h-8 w-8 place-items-center text-ink-500">
                <Icon class="h-4 w-4" />
              </span>
            ))}
          </div>
        </header>

        {/* Top-anchored: the transcript fills downward from the header the way
            a thread opened at its first message does. */}
        <div class="relative flex min-h-0 flex-1 flex-col overflow-hidden px-3 pb-6 pt-4 sm:px-5 md:px-8 md:pt-7">
          <div class="mx-auto w-full max-w-[54rem]">
            <MessageSkeleton />
          </div>
        </div>

        <div class="codex-composer-shell relative z-20 flex-none bg-canvas">
          <div class="codex-composer-card mx-3 mb-3 rounded-panel border border-line bg-surface shadow-pop">
            <div class="codex-composer-form composer-form flex flex-col px-2.5 pt-2">
              <div class="flex min-h-[2.75rem] flex-col justify-center px-1.5">
                <Skeleton class="h-2.5 w-[44%] max-w-[22rem]" />
              </div>
              <div class="codex-composer-control-deck flex min-w-0 items-center gap-1.5 pt-1.5">
                <span
                  class="grid h-8 w-8 flex-none place-items-center text-ink-500"
                  aria-hidden="true"
                >
                  <Plus class="h-4 w-4" />
                </span>
                <div class="hidden min-w-0 flex-1 items-center gap-1.5 md:flex">
                  <Skeleton class="h-7 w-[8.5rem] rounded-control" />
                  <span class="h-4 w-px flex-none bg-line-strong" aria-hidden="true" />
                  <Skeleton class="h-7 w-[6rem] rounded-control" />
                </div>
                <div class="flex-1 md:hidden" />
                <span
                  class="grid h-8 w-8 flex-none place-items-center rounded-full bg-tint-strong text-ink-500"
                  aria-hidden="true"
                >
                  <ArrowUp class="h-4 w-4" />
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
