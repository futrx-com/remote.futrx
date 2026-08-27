import type { ProjectMeta } from "../../models/project";

// Mirrors backend project.MaxSlugLen so the preview matches what the server
// will actually create.
const MAX_SLUG_LEN = 32;

export interface CreateProjectValidation {
  ok: boolean;
  slug: string;
  // Error text when ok is false; informational "Saved as <slug>" when the
  // slug differs from what was typed.
  message: string;
}

class CreateProjectFormLogic {
  readonly maxSlugLen = MAX_SLUG_LEN;

  // Mirrors backend Slugify (service/project/slug.go), except the empty
  // result stays empty here so validation can reject symbol-only names
  // instead of silently previewing the server's "project" fallback.
  slugify(name: string): string {
    let out = "";
    let lastDash = true;
    for (const r of name.trim().toLowerCase()) {
      if ((r >= "a" && r <= "z") || (r >= "0" && r <= "9")) {
        out += r;
        lastDash = false;
      } else if (r === "-" || r === "_" || r === "." || r === "/" || /\s/.test(r)) {
        if (!lastDash && out.length > 0) {
          out += "-";
          lastDash = true;
        }
      }
    }
    out = out.replace(/-+$/, "");
    if (out === "") return "";
    if (!(out[0] >= "a" && out[0] <= "z")) out = "p-" + out;
    if (out.length > MAX_SLUG_LEN) out = out.slice(0, MAX_SLUG_LEN).replace(/-+$/, "");
    return out;
  }

  // Mirrors fileproject.Store.resolveSlugLocked so distinct display names
  // whose slugs collide preview the same numbered slug the backend allocates.
  private availableSlug(base: string, projects: ProjectMeta[]): string | null {
    const taken = new Set(projects.map((project) => project.slug));
    if (!taken.has(base)) return base;

    for (let i = 2; i < 1000; i += 1) {
      const suffix = `-${i}`;
      const stem = base.slice(0, MAX_SLUG_LEN - suffix.length);
      const candidate = `${stem}${suffix}`;
      if (!taken.has(candidate)) return candidate;
    }
    return null;
  }

  validate(name: string, projects: ProjectMeta[]): CreateProjectValidation {
    const trimmed = name.trim();
    const base = this.slugify(name);
    if (!trimmed) return { ok: false, slug: base, message: "" };
    if (base.length < 2) {
      return { ok: false, slug: base, message: "Use at least 2 letters or numbers." };
    }
    const duplicateName = projects.some(
      (project) => project.name.trim().toLowerCase() === trimmed.toLowerCase()
    );
    if (duplicateName) {
      return { ok: false, slug: base, message: `A project named ${base} already exists.` };
    }
    const slug = this.availableSlug(base, projects);
    if (!slug) {
      return { ok: false, slug: base, message: "Could not find an available project name." };
    }
    return { ok: true, slug, message: slug !== trimmed ? `Saved as ${slug}` : "" };
  }

  // A project's cwd is "<root>/<slug>/workspace"; borrow an existing
  // project's cwd to preview where the new workspace will land.
  pathPreview(projects: ProjectMeta[], slug: string): string {
    const sample = projects.find(
      (project) => project.slug && project.cwd.endsWith(`/${project.slug}/workspace`)
    );
    if (!sample) return slug ? `~/projects/${slug}` : "~/projects/…";
    const root = sample.cwd.slice(0, sample.cwd.length - `/${sample.slug}/workspace`.length);
    return `${root}/${slug || "…"}/workspace`;
  }
}

export const createProjectForm = new CreateProjectFormLogic();
