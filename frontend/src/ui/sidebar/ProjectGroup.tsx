import { useLayoutEffect, useRef, useState } from "preact/hooks";
import type { ChatMeta } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";
import type { DropPosition } from "../../models/workspace";
import { ChevronDown, ChevronRight, Loader, Plus, Settings } from "../primitives/icons";
import { ChatRow } from "./ChatRow";

const projectActionClass =
  "grid h-7 w-7 place-items-center rounded-control text-ink-400 transition-colors " +
  "hover:bg-tint-strong hover:text-ink-50";

export function ProjectGroup({
  project,
  chats,
  activeChatId,
  collapsed,
  onToggle,
  onNewChat,
  onOpenContainer,
  onSelectChat,
  onDeleteChat,
  onToggleChatUnread,
  onForkChat,
  draggable,
  dragging,
  dropPosition,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
  onDragEnd,
}: {
  project: ProjectMeta;
  chats: ChatMeta[];
  activeChatId: string | null;
  collapsed: boolean;
  onToggle: () => void;
  onNewChat: () => void;
  onOpenContainer: () => void;
  onSelectChat: (chatId: string) => void;
  onDeleteChat: (chat: ChatMeta, event: Event) => void;
  onToggleChatUnread: (chat: ChatMeta, event: Event) => void;
  onForkChat: (chat: ChatMeta, event: Event) => void;
  draggable?: boolean;
  dragging?: boolean;
  dropPosition?: DropPosition | null;
  onDragStart?: (event: DragEvent) => void;
  onDragOver?: (event: DragEvent) => void;
  onDragLeave?: (event: DragEvent) => void;
  onDrop?: (event: DragEvent) => void;
  onDragEnd?: (event: DragEvent) => void;
}) {
  const provisioning = project.status === "provisioning";
  const hasUnread = chats.some((chat) => (chat.lastMessageAt || 0) > (chat.lastReadAt || 0));
  const listRef = useRef<HTMLDivElement | null>(null);
  const [moreBelow, setMoreBelow] = useState(false);

  // The clipped list gives no hint that chats continue past the fold, so we
  // track the remaining scroll distance and fade the last visible row.
  useLayoutEffect(() => {
    const node = listRef.current;
    if (collapsed || !node) {
      setMoreBelow(false);
      return;
    }
    const measure = () =>
      setMoreBelow(node.scrollHeight - node.scrollTop - node.clientHeight > 1);
    measure();
    node.addEventListener("scroll", measure, { passive: true });
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => {
      node.removeEventListener("scroll", measure);
      observer.disconnect();
    };
  }, [collapsed, chats.length]);

  return (
    <div
      class={`relative min-h-0 rounded-card transition ${dragging ? "opacity-40" : ""}`}
      onDragOver={onDragOver as any}
      onDragLeave={onDragLeave as any}
      onDrop={onDrop as any}
    >
      {/* A line in the gap, not a tint on the whole group: it says where the
          project lands, which a block highlight cannot express. */}
      {dropPosition && (
        <div
          aria-hidden="true"
          class={`pointer-events-none absolute inset-x-1 z-10 h-0.5 rounded-full bg-accent-blue
                  ${dropPosition === "before" ? "-top-px" : "-bottom-px"}`}
        />
      )}

      <div
        class={`group/head flex items-center rounded-control pr-1 hover:bg-tint
                ${draggable ? "cursor-grab active:cursor-grabbing" : ""}`}
        draggable={draggable}
        onDragStart={onDragStart as any}
        onDragEnd={onDragEnd as any}
      >
        <button
          type="button"
          onClick={onToggle}
          class="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 pl-1 pr-1 text-left"
          aria-expanded={!collapsed}
          aria-label={collapsed ? `Expand ${project.name}` : `Collapse ${project.name}`}
        >
          <span class="grid h-4 w-4 flex-none place-items-center text-ink-400">
            {collapsed ? <ChevronRight class="h-3.5 w-3.5" /> : <ChevronDown class="h-3.5 w-3.5" />}
          </span>
          <span class="min-w-0 flex-1 truncate text-[11px] font-semibold uppercase tracking-[0.08em] text-ink-300">
            {project.name}
          </span>
          {hasUnread && (
            <span
              class="h-1.5 w-1.5 flex-none rounded-full bg-accent-green"
              title="Unread chats"
            />
          )}
          {provisioning && <Loader class="h-3 w-3 flex-none animate-spin text-accent-yellow" />}
        </button>

        {/* Project-level actions stay out of the way until the header is touched. */}
        <div class="flex flex-none items-center gap-0.5 md:opacity-0 md:transition-opacity md:group-hover/head:opacity-100 md:group-focus-within/head:opacity-100">
          <button
            type="button"
            onClick={(event) => {
              event.stopPropagation();
              onOpenContainer();
            }}
            class={projectActionClass}
            aria-label={`Open container info for ${project.name}`}
            title="Container info"
          >
            <Settings class="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={(event) => {
              event.stopPropagation();
              onNewChat();
            }}
            disabled={provisioning}
            class={`${projectActionClass} disabled:cursor-not-allowed disabled:opacity-40`}
            aria-label="New chat in project"
            title={provisioning ? "Project is still provisioning" : "New chat in this project"}
          >
            <Plus class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {project.errorMsg && (
        <div
          class="mx-1 mt-1 line-clamp-2 rounded-control border border-accent-red/25 bg-accent-red/[0.08] px-2 py-1.5 font-mono text-[11px] leading-relaxed text-accent-red break-words"
          title={project.errorMsg}
        >
          {project.errorMsg}
        </div>
      )}

      {!collapsed && (
        <div class="sidebar-project-chat-frame relative mb-1 mt-1.5">
        <div
          ref={listRef}
          class="sidebar-project-chat-list space-y-px overflow-y-auto pl-3 pr-0.5 touch-scroll scrollbar-thin"
        >
          {chats.length === 0 ? (
            <button
              type="button"
              onClick={onNewChat}
              disabled={provisioning}
              class="mb-1 ml-2 inline-flex h-7 items-center gap-1.5 rounded-control px-2
                     text-[12px] text-ink-400 transition-colors hover:bg-tint hover:text-ink-100
                     disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Plus class="h-3.5 w-3.5" /> New chat
            </button>
          ) : (
            chats.map((chat) => (
              <ChatRow
                key={chat.id}
                chat={chat}
                active={chat.id === activeChatId}
                onSelect={() => onSelectChat(chat.id)}
                onDelete={(event) => onDeleteChat(chat, event)}
                onToggleUnread={(event) => onToggleChatUnread(chat, event)}
                onFork={(event) => onForkChat(chat, event)}
              />
            ))
          )}
        </div>
        {moreBelow && <div class="sidebar-project-chat-fade" aria-hidden="true" />}
        </div>
      )}
    </div>
  );
}
