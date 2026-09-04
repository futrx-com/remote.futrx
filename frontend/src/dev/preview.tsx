// Scratch harness for visual review only. Not referenced by the app entry.
import { render } from "preact";
import { useRef, useState } from "preact/hooks";
import type { ChatMeta } from "../models/chat";
import type { ProjectMeta } from "../models/project";
import type { ChatMessageBlock } from "../models/chatMessage";
import type { WorkspaceSidebarModel } from "../models/workspace";
import { AppShell } from "../ui/layout/AppShell";
import { Sidebar } from "../ui/sidebar/Sidebar";
import { useWorkspaceSearch } from "../state/hooks/workspace/useWorkspaceSearch";
import { paletteSearchStore } from "../state/stores/workspace/workspaceSearchStore";
import { ThreadHeader } from "../ui/chat/header/ThreadHeader";
import { WorkspaceActions } from "../ui/chat/header/WorkspaceActions";
import { MessageList } from "../ui/chat/messages/MessageList";
import { AttachButton } from "../ui/chat/composer/AttachButton";
import { SendControls } from "../ui/chat/composer/SendControls";
import { ComposerOptionDropdown } from "../ui/chat/composer/ComposerOptionDropdown";
import { PromptTextarea } from "../ui/chat/composer/PromptTextarea";
import { Activity, Bot, ChevronDown, Cpu, MessageSquare } from "../ui/primitives/icons";
import "../index.css";

const now = Date.now();
const hours = (n: number) => now - n * 3600_000;

function project(id: string, name: string, order: number): ProjectMeta {
  return {
    id, name, slug: name.toLowerCase().replace(/\s+/g, "-"),
    cwd: `/opt/remote.futrx/${id}`, containerName: `rf-${id}`,
    status: "running", order, createdAt: now, updatedAt: now,
  };
}

function chat(id: string, title: string, at: number, extra: Partial<ChatMeta> = {}): ChatMeta {
  return {
    id, title, provider: "claude", model: "claude-opus-5",
    createdAt: at, lastMessageAt: at, lastReadAt: at, ...extra,
  };
}

const projects = [
  project("gamerhead", "gamerhead", 0),
  project("futrx-web", "futrx web", 1),
];

const sidebarModel: WorkspaceSidebarModel = {
  visibleProjects: [
    {
      project: projects[0],
      chats: [
        chat("c1", "Refactor and commit pending changes", hours(4), { running: true }),
        chat("c2", "Professional match chat rendering", hours(29)),
        chat("c3", "Team page 404 on click", hours(52), { lastReadAt: 0 }),
        chat("c4", "Fix match detail page description overflow", hours(96)),
        chat("c5", "Right rail widget overlaps the footer", hours(120)),
      ],
    },
    {
      project: projects[1],
      chats: [
        chat("c6", "Supabase integration sweep", hours(3210)),
        chat("c7", "User custom styles and theming", hours(3240)),
      ],
    },
  ],
  visibleLooseChats: [chat("c8", "Scratch: token migration notes", hours(8))],
  totalChats: 8,
  totalProjects: 2,
};

const blocks: ChatMessageBlock[] = [
  { type: "user", t: hours(2), text: "The queue worker keeps restarting after the deploy. Can you find the actual fatal?" },
  {
    type: "assistant", t: hours(2), isComplete: true,
    parts: [
      { kind: "text", text: "## Where to look\n\nThe restart loop hides the first failure, so read the journal before the service recycles again:" },
      {
        kind: "tool", id: "t1", name: "Bash", status: "done",
        input: { command: "journalctl -u gamerhead-queue -n 100 --no-pager" },
        output: "-- Logs begin at Tue 2026-08-25 --\nAug 28 04:11:02 gamerhead systemd[1]: Started gamerhead queue worker.\nAug 28 04:11:09 gamerhead php[24812]: PHP Fatal error:  Uncaught RedisException: Connection refused",
      },
      {
        kind: "tool", id: "t2", name: "Read", status: "done",
        input: { file_path: "/var/www/gamerhead/api/config/queue.php" },
        output: "'default' => env('QUEUE_CONNECTION', 'redis'),",
      },
      { kind: "text", text: "### Meanwhile\n\nThe web health check passed and `current` was switched before the restarts, so the site is serving the new release — this isn't a site-down. The impact is confined to the queue: `VerificationMail`, `PasswordResetMail` and `NotificationMail` are `ShouldQueue`, so signup verifications are accumulating in the `jobs` table and are not being delivered until the worker is back." },
    ],
  },
  { type: "user", t: hours(1), text: "Paste the journal output and I'll work the actual error." },
  {
    type: "assistant", t: hours(1), isComplete: false,
    parts: [{ kind: "text", text: "Running the read-only check now so nothing is consumed from the queue." }],
  },
];

const noop = () => {};
const noopAsync = async () => {};

// Every chat the fake model shows, so the real search hook has something to
// rank when the search box in this harness is used.
const previewChats = [
  ...sidebarModel.visibleProjects.flatMap((node) => node.chats),
  ...sidebarModel.visibleLooseChats,
];

function Preview() {
  // The palette's store rather than the sidebar's: it starts from the
  // defaults and saves nothing, so this harness cannot overwrite the filters
  // the real sidebar remembers.
  const search = useWorkspaceSearch(paletteSearchStore, previewChats, projects);
  const [text, setText] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const workspaceActions = {
    cwd: "/opt/remote.futrx/gamerhead",
    onToggleTerminal: noop, onToggleBrowser: noop, onToggleHistory: noop,
    onToggleFiles: noop, onToggleSchedules: noop,
    terminalOpen: false, browserOpen: true, historyOpen: false,
    filesOpen: false, schedulesOpen: false, showHistory: true, showSchedules: true,
    orientation: "horizontal" as const,
  };

  return (
    <AppShell
      sidebar={
        <Sidebar
          open={false}
          model={sidebarModel}
          loading={false}
          search={search}
          collapsed={{}}
          sidebarCollapsed={false}
          activeChatId="c1"
          account={{ email: "me@ahmedwaleed.net", authenticated: true }}
          onClose={noop} onOpenPalette={noop} onToggleSidebar={noop}
          onNewProject={noop} onNewChatInProject={noop} onToggleProject={noop}
          onSelectChat={noop} onDeleteChat={noop} onToggleChatUnread={noop} onForkChat={noop}
          onReorderProjects={noop} onOpenProjectContainers={noop} onOpenSettings={noop} onSignOut={noop}
        />
      }
    >
      <div class="codex-thread flex h-full min-h-0 flex-1 overflow-hidden bg-canvas">
        <div class="flex min-w-0 flex-1 flex-col">
          <ThreadHeader
            chat={chat("c1", "Refactor and commit pending changes", hours(4))}
            streaming
            projectName="gamerhead"
            actions={<WorkspaceActions {...workspaceActions} />}
            onHamburger={noop}
          />
          <div class="workspace-action-toolbar relative z-30 flex flex-none justify-end border-b border-line px-2.5 py-1.5 md:hidden">
            <WorkspaceActions {...workspaceActions} />
          </div>
          <div class="relative min-h-0 flex-1">
            <MessageList
              status="streaming" blocks={blocks} hasOlder loadingOlder={false} error={null}
              chatId="c1" cwd="/opt/remote.futrx/gamerhead"
              scrollRef={scrollRef} contentRef={contentRef} bottomRef={bottomRef}
              onScroll={noop} onAnswerQuestion={noop} onLoadOlder={noopAsync} onRewind={noop}
            />
          </div>

          <div class="codex-composer-shell relative z-20 flex-none bg-canvas">
            <div class="codex-composer-card mx-3 mb-3 overflow-visible rounded-panel border border-line bg-surface shadow-pop">
              <div class="codex-composer-form composer-form flex flex-col px-2.5 pt-2">
                <PromptTextarea
                  textareaRef={textareaRef} text={text} uploading={false} streaming={false}
                  disconnected={false} onTextChange={setText} onPaste={noop} onSend={noop}
                />
                <div class="codex-composer-control-deck flex min-w-0 items-center gap-1.5 pt-1.5">
                  <AttachButton fileInputRef={fileInputRef} uploading={false} disconnected={false} onFilesSelected={noop} />
                  <div class="hidden min-w-0 flex-1 items-center gap-1.5 md:flex">
                    <div class="codex-composer-agent-controls flex min-w-0 flex-wrap items-center gap-1">
                      <button
                        type="button"
                        class="inline-flex h-7 items-center gap-1.5 rounded-control px-2 text-[11.5px] text-ink-300 transition-colors hover:bg-tint-strong hover:text-ink-100"
                      >
                        <Bot class="h-3 w-3 opacity-60" />
                        <span class="font-medium">Claude Opus 5</span>
                        <ChevronDown class="h-3 w-3 opacity-50" />
                      </button>
                    </div>
                    <span class="h-4 w-px flex-none bg-line-strong" />
                    <div class="codex-composer-execution-controls flex min-w-0 flex-wrap items-center gap-1">
                      <ComposerOptionDropdown label="Thinking" value="high" Icon={Activity}
                        options={[{ value: "high", label: "High" }, { value: "max", label: "Max" }]} onChange={noop} />
                      <ComposerOptionDropdown label="Speed" value="auto" Icon={Cpu}
                        options={[{ value: "auto", label: "Auto" }, { value: "flex", label: "Flex" }]} onChange={noop} />
                      <ComposerOptionDropdown label="Mode" value="auto" Icon={MessageSquare}
                        options={[{ value: "auto", label: "Auto" }, { value: "plan", label: "Plan" }]} onChange={noop} />
                    </div>
                  </div>
                  <SendControls streaming={false} canSend={text.length > 0} disconnected={false} onCancel={noop} />
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </AppShell>
  );
}

document.documentElement.dataset.theme =
  new URLSearchParams(location.search).get("theme") === "light" ? "light" : "dark";
render(<Preview />, document.getElementById("root")!);
