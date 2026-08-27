// WorkspaceActions — icon-button rail (IDE, terminal, history, files, schedules, browser).
import { WorkspaceActions } from "remote.futrx-web";

const noop = () => {};

const baseProps = {
  cwd: "/opt/remote.futrx/projects/storefront",
  onOpenTerminal: noop,
  onToggleBrowser: noop,
  onToggleHistory: noop,
  onToggleFiles: noop,
  onToggleSchedules: noop,
};

export const HorizontalRail = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <WorkspaceActions
      {...baseProps}
      browserOpen={false}
      historyOpen={false}
      filesOpen={false}
      schedulesOpen={false}
      showHistory
      showSchedules
      orientation="horizontal"
    />
  </div>
);

export const ActivePanes = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <WorkspaceActions
      {...baseProps}
      browserOpen
      historyOpen={false}
      filesOpen
      schedulesOpen={false}
      showHistory
      showSchedules
      orientation="horizontal"
    />
  </div>
);

export const VerticalRail = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <WorkspaceActions
      {...baseProps}
      browserOpen={false}
      historyOpen
      filesOpen={false}
      schedulesOpen={false}
      showHistory
      showSchedules
      orientation="vertical"
    />
  </div>
);

export const MinimalSet = () => (
  <div className="w-full max-w-xl flex justify-end items-start">
    <WorkspaceActions
      {...baseProps}
      cwd="~"
      browserOpen={false}
      historyOpen={false}
      filesOpen={false}
      schedulesOpen={false}
      showHistory={false}
      showSchedules={false}
      orientation="horizontal"
    />
  </div>
);
