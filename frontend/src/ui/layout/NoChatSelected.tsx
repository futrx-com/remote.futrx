import { Folder, Menu, MessageSquare, Plus } from "../primitives/icons";

export function NoChatSelected({
  hasProjects,
  onNewProject,
  onNewChat,
  onHamburger,
}: {
  hasProjects: boolean;
  onNewProject: () => void;
  onNewChat: () => void;
  onHamburger: () => void;
}) {
  return (
    <div class="flex-1 flex flex-col min-h-0">
      <header class="codex-header top-chrome z-20 flex min-h-[46px] flex-none items-center gap-2 border-b border-line px-2.5 pb-2">
        <button
          type="button"
          onClick={onHamburger}
          class="md:hidden h-10 w-10 text-ink-100 rounded-md hover:bg-tint-strong grid place-items-center"
          aria-label="Toggle sidebar"
        >
          <Menu class="w-5 h-5" />
        </button>
        <span class="text-sm text-ink-200">No chat selected</span>
      </header>
      <div class="flex-1 grid place-items-center text-center p-5">
        <div class="space-y-5 max-w-sm">
          <div class="mx-auto w-16 h-16 rounded-lg bg-tint border border-line grid place-items-center">
            {hasProjects ? (
              <MessageSquare class="w-8 h-8 opacity-70" />
            ) : (
              <Folder class="w-8 h-8 opacity-70" />
            )}
          </div>
          <div class="text-ink-200">
            <div class="font-semibold text-lg text-ink-50">
              {hasProjects ? "Choose a chat or start fresh" : "Create your first project"}
            </div>
            <div class="text-xs mt-2 leading-relaxed text-ink-300">
              {hasProjects
                ? "Pick a project on the left, then create a chat inside it. Each project is its own sandboxed container."
                : "Projects are sandboxed dev environments. Agent CLIs install and run inside each project's container, isolated from the rest of the host."}
            </div>
          </div>
          <div class="flex gap-2 justify-center">
            <button
              type="button"
              onClick={onNewProject}
              class="btn btn-primary btn-lg"
            >
              <Folder class="w-4 h-4" /> New project
            </button>
            {hasProjects && (
              <button
                type="button"
                onClick={onNewChat}
                class="btn btn-secondary btn-lg"
              >
                <Plus class="w-4 h-4" /> Loose chat
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
