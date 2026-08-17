import type { ComponentType } from "preact";
import { Code, File, Globe, Layers, Package } from "../primitives/icons";

/**
 * Maps a template's `icon` key to a glyph. The key is data shipped by the
 * backend, so an unknown one (a template added by a newer backend, or a
 * custom template) must render rather than crash — hence the fallback.
 */
const glyphs: Record<string, ComponentType<{ class?: string }>> = {
  blank: File,
  wordpress: Globe,
  laravel: Layers,
  node: Package,
  python: Code,
};

export function templateIcon(icon: string): ComponentType<{ class?: string }> {
  return glyphs[icon] ?? File;
}
