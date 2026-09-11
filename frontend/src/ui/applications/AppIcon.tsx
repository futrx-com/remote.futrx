// The mark shown for a catalog image, in the grid and on installed rows.
//
// An image chooses it with `icon` in image.json, in one of two forms:
//
//   "database"           a built-in key from the table below
//   "ui/assets/logo.svg" a file the image ships in its own ui/ directory
//
// The second form is what lets an image bring its own artwork without adding
// anything to the SPA's icon set; it is served through the same authenticated
// asset route as the rest of the extension.

import type { AppImage } from "../../models/application";
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

/** Built-in marks an image may name. Anything unknown falls back to Server. */
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

export function AppIcon({
  image,
  class: className = "w-4 h-4",
}: {
  image: Pick<AppImage, "id" | "icon" | "name">;
  class?: string;
}) {
  const icon = image.icon?.trim() ?? "";

  const Icon = BUILT_IN[icon.toLowerCase()] ?? Server;
  return <Icon class={className} />;
}
