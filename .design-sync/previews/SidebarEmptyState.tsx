// SidebarEmptyState — shown when the workspace has no projects at all.
import { SidebarEmptyState } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <SidebarEmptyState onNewProject={() => {}} />
  </div>
);

export const NarrowRail = () => (
  <div className="w-full" style={{ maxWidth: "240px" }}>
    <SidebarEmptyState onNewProject={() => {}} />
  </div>
);
