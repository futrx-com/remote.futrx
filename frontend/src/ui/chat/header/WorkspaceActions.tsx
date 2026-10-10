import type { ComponentChildren } from "preact";
import { useId, useState } from "preact/hooks";
import { useDismissKeyDown } from "../../../state/hooks/shared/useDismissKeyDown.ts";
import { Clock, Folder, Monitor } from "../../primitives/icons";
import { ExtensionSlot } from "../../primitives/ExtensionSlot";
import { workspaceActionTooltipClass } from "../../primitives/workspaceActionTooltip.ts";
import { EXTENSION_SLOTS } from "../../../config/extensions";
import { DEFAULT_WORKSPACE_PATH } from "../../../config/workspace";
import { extensionDrawerState } from "../../../state/hooks/chat/extensionDrawerState";

// Two states only, and they never fight over the same property: Tailwind emits
// utilities in file order, so an "expanded" colour appended after a base colour
// would silently lose. Pick one complete class string instead.
const actionBase =
  "workspace-action relative inline-flex h-8 w-8 flex-none items-center justify-center " +
  "rounded-control transition-colors";
const actionIdle = `${actionBase} text-ink-400 hover:bg-tint-strong hover:text-ink-50`;
const actionIcon = "h-4 w-4 flex-none";

/** One drawer an installed application docks beside the chat. */
export interface ExtensionDrawerAction {
  id: string;
  label: string;
  /** Inline SVG supplied by the application. */
  icon: string;
  open: boolean;
  onToggle: () => void;
}

const actionExpanded = `${actionBase} bg-accent-blue/[0.14] text-accent-blue hover:bg-accent-blue/20`;

export function WorkspaceActions({
  cwd,
  chatId,
  projectId,
  extensionDrawers,
  onToggleBrowser,
  onToggleHistory,
  onToggleFiles,
  browserOpen,
  historyOpen,
  filesOpen,
  showHistory,
  orientation,
}: {
  cwd: string;
  chatId?: string;
  projectId?: string;
  extensionDrawers: ExtensionDrawerAction[];
  onToggleBrowser: () => void;
  onToggleHistory: () => void;
  onToggleFiles: () => void;
  browserOpen: boolean;
  historyOpen: boolean;
  filesOpen: boolean;
  showHistory: boolean;
  orientation: "horizontal" | "vertical";
}) {
  const workspacePath = cwd && cwd !== "~" ? cwd : DEFAULT_WORKSPACE_PATH;
  const tooltipPlacement = orientation === "horizontal" ? "below" : "left";

  return (
    <div class={`flex items-center gap-0.5 ${orientation === "horizontal" ? "flex-row" : "flex-col"}`}>
      <ExtensionSlot
        name={EXTENSION_SLOTS.chatHeaderActions}
        chatId={chatId}
        projectId={projectId}
        cwd={workspacePath}
        tooltipPlacement={tooltipPlacement}
      />
      {extensionDrawers.map((drawer) => (
        <WorkspaceAction
          key={drawer.id}
          icon={
            <span
              aria-hidden="true"
              class={`grid place-items-center ${actionIcon} [&>svg]:h-full [&>svg]:w-full`}
              dangerouslySetInnerHTML={{ __html: drawer.icon }}
            />
          }
          onClick={drawer.onToggle}
          label={drawer.open ? `Close ${drawer.label}` : drawer.label}
          tooltip={drawer.open ? `Close ${drawer.label}` : `Open ${drawer.label}`}
          expanded={drawer.open}
          controls={extensionDrawerState.paneElementId(drawer.id)}
          action={extensionDrawerState.paneName(drawer.id)}
          tooltipPlacement={tooltipPlacement}
        />
      ))}
      {showHistory && (
        <WorkspaceAction
          icon={<Clock aria-hidden="true" focusable="false" class={actionIcon} />}
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
        icon={<Folder aria-hidden="true" focusable="false" class={actionIcon} />}
        onClick={onToggleFiles}
        label={filesOpen ? "Close workspace files" : "Workspace files"}
        tooltip={filesOpen ? "Close workspace files" : "Browse workspace files"}
        expanded={filesOpen}
        controls="workspace-files-pane"
        action="files"
        tooltipPlacement={tooltipPlacement}
      />
      <WorkspaceAction
        icon={<Monitor aria-hidden="true" focusable="false" class={actionIcon} />}
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
  icon,
  label,
  tooltip,
  onClick,
  expanded,
  controls,
  action,
  tooltipPlacement,
}: {
  icon: ComponentChildren;
  label: string;
  tooltip: string;
  onClick?: () => void;
  expanded?: boolean;
  controls?: string;
  action?: string;
  tooltipPlacement: "below" | "left";
}) {
  const tooltipId = useId();
  const [isHovered, setIsHovered] = useState(false);
  const [isFocused, setIsFocused] = useState(false);
  const [isDismissed, setIsDismissed] = useState(false);
  const onKeyDown = useDismissKeyDown((event) => {
    setIsDismissed(true);
    event.stopPropagation();
  });
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
    onKeyDown,
    onMouseEnter: () => {
      setIsHovered(true);
      setIsDismissed(false);
    },
    onMouseLeave: () => setIsHovered(false),
  };
  const content = (
    <>
      {icon}
      <span
        id={tooltipId}
        role="tooltip"
        class={workspaceActionTooltipClass(tooltipPlacement, isTooltipOpen)}
      >
        {tooltip}
      </span>
    </>
  );

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
