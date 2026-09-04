import { useId, useState } from "preact/hooks";
import { CalendarClock, Clock, Code, Folder, Monitor, Terminal } from "../../primitives/icons";
import { buildIdeUrl, defaultWorkspacePath } from "../ideLinks";

// Two states only, and they never fight over the same property: Tailwind emits
// utilities in file order, so an "expanded" colour appended after a base colour
// would silently lose. Pick one complete class string instead.
const actionBase =
  "workspace-action relative inline-flex h-8 w-8 flex-none items-center justify-center " +
  "rounded-control transition-colors";
const actionIdle = `${actionBase} text-ink-400 hover:bg-tint-strong hover:text-ink-50`;
const actionExpanded = `${actionBase} bg-accent-blue/[0.14] text-accent-blue hover:bg-accent-blue/20`;

export function WorkspaceActions({
  cwd,
  onToggleTerminal,
  onToggleBrowser,
  onToggleHistory,
  onToggleFiles,
  onToggleSchedules,
  terminalOpen,
  browserOpen,
  historyOpen,
  filesOpen,
  schedulesOpen,
  showHistory,
  showSchedules,
  orientation,
}: {
  cwd: string;
  onToggleTerminal: () => void;
  onToggleBrowser: () => void;
  onToggleHistory: () => void;
  onToggleFiles: () => void;
  onToggleSchedules: () => void;
  terminalOpen: boolean;
  browserOpen: boolean;
  historyOpen: boolean;
  filesOpen: boolean;
  schedulesOpen: boolean;
  showHistory: boolean;
  showSchedules: boolean;
  orientation: "horizontal" | "vertical";
}) {
  const workspacePath = cwd && cwd !== "~" ? cwd : defaultWorkspacePath;
  const ideUrl = buildIdeUrl(workspacePath);
  const tooltipPlacement = orientation === "horizontal" ? "below" : "left";

  return (
    <div class={`flex items-center gap-0.5 ${orientation === "horizontal" ? "flex-row" : "flex-col"}`}>
      <WorkspaceAction
        Icon={Code}
        href={ideUrl}
        label="Workspace IDE"
        tooltip="Open workspace in IDE"
        tooltipPlacement={tooltipPlacement}
      />
      <WorkspaceAction
        Icon={Terminal}
        onClick={onToggleTerminal}
        label={terminalOpen ? "Close container terminal" : "Container terminal"}
        tooltip={terminalOpen ? "Close container terminal" : "Open container terminal"}
        expanded={terminalOpen}
        controls="workspace-terminal-pane"
        action="terminal"
        tooltipPlacement={tooltipPlacement}
      />
      {showHistory && (
        <WorkspaceAction
          Icon={Clock}
          onClick={onToggleHistory}
          label={historyOpen ? "Close git history" : "Git history"}
          tooltip={historyOpen ? "Close git history" : "Review git history"}
          expanded={historyOpen}
          controls="workspace-history-pane"
          action="history"
          tooltipPlacement={tooltipPlacement}
        />
      )}
      <WorkspaceAction
        Icon={Folder}
        onClick={onToggleFiles}
        label={filesOpen ? "Close workspace files" : "Workspace files"}
        tooltip={filesOpen ? "Close workspace files" : "Browse workspace files"}
        expanded={filesOpen}
        controls="workspace-files-pane"
        action="files"
        tooltipPlacement={tooltipPlacement}
      />
      {showSchedules && (
        <WorkspaceAction
          Icon={CalendarClock}
          onClick={onToggleSchedules}
          label={schedulesOpen ? "Close scheduled tasks" : "Scheduled tasks"}
          tooltip={schedulesOpen ? "Close scheduled tasks" : "View scheduled tasks"}
          expanded={schedulesOpen}
          controls="workspace-schedules-pane"
          action="schedules"
          tooltipPlacement={tooltipPlacement}
        />
      )}
      <WorkspaceAction
        Icon={Monitor}
        onClick={onToggleBrowser}
        label={browserOpen ? "Close browser preview" : "Browser preview"}
        tooltip={browserOpen ? "Close browser preview" : "Open browser preview"}
        expanded={browserOpen}
        controls="workspace-browser-pane"
        action="browser"
        tooltipPlacement={tooltipPlacement}
      />
    </div>
  );
}

function WorkspaceAction({
  Icon,
  label,
  tooltip,
  href,
  onClick,
  expanded,
  controls,
  action,
  tooltipPlacement,
}: {
  Icon: typeof Code;
  label: string;
  tooltip: string;
  href?: string;
  onClick?: () => void;
  expanded?: boolean;
  controls?: string;
  action?: "history" | "files" | "schedules" | "browser" | "terminal";
  tooltipPlacement: "below" | "left";
}) {
  const tooltipId = useId();
  const [isHovered, setIsHovered] = useState(false);
  const [isFocused, setIsFocused] = useState(false);
  const [isDismissed, setIsDismissed] = useState(false);
  const isTooltipOpen = !isDismissed && (isHovered || isFocused);
  const interactionProps = {
    "aria-describedby": tooltipId,
    "aria-label": label,
    onBlur: () => {
      setIsFocused(false);
      setIsDismissed(false);
    },
    onFocus: () => {
      setIsFocused(true);
      setIsDismissed(false);
    },
    onKeyDown: (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setIsDismissed(true);
      event.stopPropagation();
    },
    onMouseEnter: () => {
      setIsHovered(true);
      setIsDismissed(false);
    },
    onMouseLeave: () => setIsHovered(false),
  };
  const content = (
    <>
      <Icon aria-hidden="true" focusable="false" class="h-4 w-4 flex-none" />
      <span
        id={tooltipId}
        role="tooltip"
        class={`workspace-action-tooltip pointer-events-none absolute z-50 whitespace-nowrap rounded-control border border-line bg-raised px-2 py-1 text-[11px] font-medium text-ink-100 shadow-pop transition-[opacity,transform] duration-150 motion-reduce:transition-none ${
          tooltipPlacement === "below"
            ? `right-0 top-full mt-2 ${isTooltipOpen ? "translate-y-0 opacity-100" : "-translate-y-1 opacity-0"}`
            : `right-full top-1/2 mr-2 -translate-y-1/2 ${isTooltipOpen ? "translate-x-0 opacity-100" : "translate-x-1 opacity-0"}`
        }`}
      >
        {tooltip}
      </span>
    </>
  );

  if (href) {
    return (
      <a
        {...interactionProps}
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        class={actionIdle}
      >
        {content}
      </a>
    );
  }

  return (
    <button
      {...interactionProps}
      type="button"
      onClick={onClick}
      aria-expanded={expanded}
      aria-controls={controls}
      data-workspace-action={action}
      class={expanded ? actionExpanded : actionIdle}
    >
      {content}
    </button>
  );
}
