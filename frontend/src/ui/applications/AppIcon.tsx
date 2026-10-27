// The mark shown for a catalog application, in the grid and on installed rows.
//
// An application chooses it with `icon` in application.json, in one of two forms:
//
//   "database"           a built-in key from the table below
//   "ui/assets/logo.svg" a file the application ships in its own ui/ directory
//
// The second form is what lets an application bring its own artwork without adding
// anything to the SPA's icon set; it is served through the same authenticated
// asset route as the rest of the extension.

import { API_ROUTES } from "../../config/routes";
import type { AppApplication } from "../../models/application";
import {
  Activity,
  Archive,
  Bell,
  Bot,
  Boxes,
  Clock,
  Code,
  Cpu,
  Folder,
  HardDrive,
  Key,
  MemoryStick,
  Monitor,
  Network,
  Server,
  Settings,
  ShieldCheck,
  Terminal,
  Users,
} from "../primitives/icons";

type IconComponent = typeof Server;

/** Built-in marks an application may name. Anything unknown falls back to Server. */
const BUILT_IN: Record<string, IconComponent> = {
  activity: Activity,
  archive: Archive,
  bell: Bell,
  bot: Bot,
  box: Boxes,
  cache: MemoryStick,
  clock: Clock,
  code: Code,
  cpu: Cpu,
  database: HardDrive,
  disk: HardDrive,
  folder: Folder,
  key: Key,
  layers: Boxes,
  memory: MemoryStick,
  monitor: Monitor,
  network: Network,
  server: Server,
  settings: Settings,
  shield: ShieldCheck,
  terminal: Terminal,
  users: Users,
};

/** An icon value pointing at a file the application ships, rather than a key. */
function assetPath(icon: string): string | null {
  const trimmed = icon.trim();
  return trimmed.startsWith("ui/") ? trimmed.slice("ui/".length) : null;
}

export function AppIcon({
  application,
  class: className = "w-4 h-4",
}: {
  application: Pick<AppApplication, "id" | "icon" | "name">;
  class?: string;
}) {
  const icon = application.icon?.trim() ?? "";
  const asset = icon ? assetPath(icon) : null;

  if (asset) {
    return (
      <img
        src={API_ROUTES.applications.uiAsset(application.id, asset)}
        alt=""
        aria-hidden="true"
        class={`${className} object-contain`}
      />
    );
  }

  const Icon = BUILT_IN[icon.toLowerCase()] ?? Server;
  return <Icon class={className} />;
}
