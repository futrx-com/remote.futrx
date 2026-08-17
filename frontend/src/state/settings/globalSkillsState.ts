import type { GlobalSkill } from "../../models/globalSkill";
import type { RegisteredSkill } from "../../models/skill";

// The manifest every skill directory must carry, matching the backend's
// serviceskills.SkillFileName.
export const SKILL_MANIFEST_FILE = "SKILL.md";

export interface SkillManifestMetadata {
  name: string;
  description: string;
}

export interface GlobalSkillDraft {
  name: string;
  manifest: string;
  extraFiles: Record<string, string>;
  alwaysOn: boolean;
}

// GlobalSkillsState holds the pure transitions of the admin "Global skills"
// editor: reading a SKILL.md header, deriving a safe directory name, and
// keeping the library list sorted after a create, edit, or delete. Keeping
// them here (and not in the component) is what makes them testable.
class GlobalSkillsState {
  // Mirrors the backend's SKILL.md parser: the leading `---` block wins, and
  // a level-one heading is the fallback name for a manifest without one.
  parseManifest(text: string): SkillManifestMetadata {
    const metadata: SkillManifestMetadata = { name: "", description: "" };
    let inFrontMatter = false;
    let sawFrontMatter = false;

    for (const rawLine of text.split(/\r?\n/)) {
      const line = rawLine.trim();
      if (line === "---") {
        if (!sawFrontMatter) {
          sawFrontMatter = true;
          inFrontMatter = true;
          continue;
        }
        break;
      }
      if (!inFrontMatter) {
        if (line.startsWith("# ") && !metadata.name) {
          metadata.name = line.slice(2).trim();
        }
        continue;
      }
      const separator = line.indexOf(":");
      if (separator < 0) continue;
      const key = line.slice(0, separator).trim().toLowerCase();
      const value = this.unquote(line.slice(separator + 1));
      if (key === "name") metadata.name = value;
      if (key === "description") metadata.description = value;
    }
    return metadata;
  }

  // Directory names must match the backend rule: lowercase letters, digits,
  // '.', '_' and '-', at most 64 characters, and never a reserved prefix.
  normalizeName(value: string): string {
    return value.trim().toLowerCase();
  }

  isValidName(value: string): boolean {
    const name = this.normalizeName(value);
    if (!name || name.length > 64) return false;
    if (name.startsWith(".") || name.startsWith("_")) return false;
    return /^[a-z0-9._-]+$/.test(name);
  }

  // suggestName turns a human title into a directory name so the editor can
  // prefill the field from the manifest the admin just pasted.
  suggestName(title: string): string {
    const slug = title
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9._-]+/g, "-")
      .replace(/-+/g, "-")
      .replace(/^[-._]+/, "")
      .replace(/[-._]+$/, "");
    return slug.slice(0, 64);
  }

  emptyDraft(): GlobalSkillDraft {
    return {
      name: "",
      manifest: "---\nname: \ndescription: \n---\n\n",
      extraFiles: {},
      alwaysOn: false,
    };
  }

  draftFromSkill(skill: GlobalSkill): GlobalSkillDraft {
    const files = { ...(skill.files ?? {}) };
    const manifest = files[SKILL_MANIFEST_FILE] ?? "";
    delete files[SKILL_MANIFEST_FILE];
    return {
      name: skill.name,
      manifest,
      extraFiles: files,
      alwaysOn: skill.alwaysOn,
    };
  }

  // buildFiles assembles the wire payload. The manifest always wins the
  // SKILL.md slot so an extra file cannot silently replace it.
  buildFiles(draft: GlobalSkillDraft): Record<string, string> {
    return { ...draft.extraFiles, [SKILL_MANIFEST_FILE]: draft.manifest };
  }

  // validateDraft returns the first blocking problem, or null when the draft
  // can be submitted.
  validateDraft(draft: GlobalSkillDraft): string | null {
    if (!this.isValidName(draft.name)) {
      return "Name must be 1-64 characters of lowercase letters, digits, '.', '_' or '-'.";
    }
    if (!draft.manifest.trim()) return "SKILL.md cannot be empty.";
    for (const path of Object.keys(draft.extraFiles)) {
      if (!this.isValidFilePath(path)) return `Invalid file path: ${path}`;
    }
    return null;
  }

  isValidFilePath(path: string): boolean {
    const candidate = path.trim().replace(/\\/g, "/");
    if (!candidate || candidate.startsWith("/")) return false;
    if (candidate === SKILL_MANIFEST_FILE) return false;
    return candidate
      .split("/")
      .every((segment) => segment !== "" && segment !== "." && segment !== ".." && !segment.startsWith("."));
  }

  upsert(skills: GlobalSkill[], skill: GlobalSkill): GlobalSkill[] {
    const next = skills.filter((entry) => entry.name !== skill.name);
    next.push(skill);
    return next.sort((left, right) => left.name.localeCompare(right.name));
  }

  remove(skills: GlobalSkill[], name: string): GlobalSkill[] {
    return skills.filter((entry) => entry.name !== name);
  }

  // A shadowed global skill is displayed but cannot be selected: the
  // project's own directory of the same name is what the container links
  // inside the workspace, so the global copy would never be the one loaded.
  isSelectable(skill: RegisteredSkill): boolean {
    return skill.shadowed !== true;
  }

  // badgeFor renders the picker's scope chip. Global entries advertise the
  // library and, when shadowed, why they cannot be chosen.
  badgeFor(skill: RegisteredSkill): string {
    if (skill.scope !== "global") return skill.source ?? "";
    if (skill.shadowed) return "shadowed";
    if (skill.alwaysOn) return "global · always on";
    return "global";
  }

  private unquote(value: string): string {
    const trimmed = value.trim();
    if (trimmed.length >= 2) {
      const first = trimmed[0];
      const last = trimmed[trimmed.length - 1];
      if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
        return trimmed.slice(1, -1).trim();
      }
    }
    return trimmed;
  }
}

export const globalSkillsState = new GlobalSkillsState();
