// SidebarSkeleton — project list placeholder shown until the first workspace
// snapshot lands, in place of the "No projects yet" empty state.
import { SidebarSkeleton } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full px-2" style={{ maxWidth: "300px" }}>
    <SidebarSkeleton />
  </div>
);

export const NarrowRail = () => (
  <div className="w-full px-2" style={{ maxWidth: "240px" }}>
    <SidebarSkeleton />
  </div>
);
