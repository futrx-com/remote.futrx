// SidebarNoMatches — shown when a workspace search filters out everything.
import { SidebarNoMatches } from "remote.futrx-web";

export const Default = () => (
  <div className="w-full" style={{ maxWidth: "320px" }}>
    <SidebarNoMatches />
  </div>
);

export const NarrowRail = () => (
  <div className="w-full" style={{ maxWidth: "240px" }}>
    <SidebarNoMatches />
  </div>
);
