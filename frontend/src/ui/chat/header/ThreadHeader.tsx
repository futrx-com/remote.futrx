import type { ComponentChildren } from "preact";
import type { ChatMeta } from "../../../models/chat";
import { providerDisplayLabel } from "../../../config/chat";
import { ChevronRight, Menu } from "../../primitives/icons";

export function ThreadHeader({
  chat,
  streaming,
  projectName,
  actions,
  onHamburger,
}: {
  chat: ChatMeta;
  streaming: boolean;
  projectName?: string;
  actions?: ComponentChildren;
  onHamburger: () => void;
}) {
  return (
    <header class="codex-header top-chrome z-20 flex flex-none items-center gap-2 border-b border-line px-5 pb-2">
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

      {/* Breadcrumb: where you are, then what you are looking at. */}
      <div class="codex-thread-heading flex min-w-0 flex-1 items-center gap-1.5">
        {projectName && (
          <>
            <span class="hidden max-w-[10rem] truncate text-[13px] text-ink-400 sm:block">
              {projectName}
            </span>
            <ChevronRight class="hidden h-3 w-3 flex-none text-ink-500 sm:block" />
          </>
        )}
        <h1 class="min-w-0 truncate text-[13.5px] font-medium tracking-[-0.005em] text-ink-50">
          {chat.title || "Untitled chat"}
        </h1>
        <span
          class={`ml-0.5 h-1.5 w-1.5 flex-none rounded-full ${
            streaming ? "animate-pulse bg-accent-green" : "bg-ink-500"
          }`}
          title={`${providerDisplayLabel(chat.provider)} · ${streaming ? "Working" : "Ready"}`}
        />
      </div>

      {actions && <div class="hidden flex-none items-center md:flex">{actions}</div>}
    </header>
  );
}
