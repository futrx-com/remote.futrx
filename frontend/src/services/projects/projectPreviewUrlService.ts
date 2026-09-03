import {
  PROJECT_PREVIEW_PORT_RANGE,
  PROJECT_PREVIEW_URL,
} from "../../config/project.ts";

// Reads and writes the project preview URL shape declared in config/project.ts.
// The grammar itself lives there because the backend defines it; this service
// is what composes it — building those URLs, finding them in agent output, and
// checking a candidate really is one of ours before the drawer follows it.
class ProjectPreviewUrlService {
  build(slug: string, port: number | null, publicHostname: string): string {
    const hostSuffix = this.hostSuffix(publicHostname);
    if (!slug || !port || !hostSuffix) return "";
    return `${PROJECT_PREVIEW_URL.scheme}://${this.hostPrefix(slug)}${port}${hostSuffix}`;
  }

  /** Preview URLs mentioned in a block of text, trailing punctuation trimmed. */
  findInText(text: string, publicHostname: string): string[] {
    const hostname = this.normalizeHostname(publicHostname);
    if (!hostname) return [];
    const pattern = new RegExp(
      `${this.escapeRegExp(PROJECT_PREVIEW_URL.scheme)}:\\/\\/` +
        `[a-z0-9][a-z0-9-]*${this.escapeRegExp(PROJECT_PREVIEW_URL.portSeparator)}` +
        `${this.portDigitsPattern()}\\.${this.escapeRegExp(PROJECT_PREVIEW_URL.subdomain)}\\.` +
        `${this.escapeRegExp(hostname)}[^\\s<>)\\]]*`,
      "g",
    );
    return [...text.matchAll(pattern)]
      .map((match) => match[0].replace(/[.,;:!?]+$/, ""));
  }

  /** Whether `raw` is a preview URL for this project on this deployment —
   *  right scheme, right host suffix, right slug, and a port in range. */
  belongsToProject(raw: string, slug: string, publicHostname: string): boolean {
    try {
      const url = new URL(raw);
      const hostSuffix = this.hostSuffix(publicHostname);
      const hostPrefix = this.hostPrefix(slug);
      return (
        hostSuffix !== "" &&
        url.protocol === `${PROJECT_PREVIEW_URL.scheme}:` &&
        url.hostname.startsWith(hostPrefix) &&
        url.hostname.endsWith(hostSuffix) &&
        this.isValidPort(url.hostname.slice(hostPrefix.length, -hostSuffix.length))
      );
    } catch {
      return false;
    }
  }

  port(url: string): number | null {
    const pattern = new RegExp(
      `${this.escapeRegExp(PROJECT_PREVIEW_URL.portSeparator)}(${this.portDigitsPattern()})\\.`,
    );
    const match = pattern.exec(url);
    return match ? Number(match[1]) : null;
  }

  /** `<slug>--`, the part of the hostname that comes before the port. */
  private hostPrefix(slug: string): string {
    return `${slug}${PROJECT_PREVIEW_URL.portSeparator}`;
  }

  /** `.dev.<public hostname>`, the part that comes after it. */
  private hostSuffix(publicHostname: string): string {
    const hostname = this.normalizeHostname(publicHostname);
    return hostname ? `.${PROJECT_PREVIEW_URL.subdomain}.${hostname}` : "";
  }

  /** A coarse `\d{4,5}` pre-filter derived from the range, so the pattern can
   *  never drift from the bound `isValidPort` actually enforces. */
  private portDigitsPattern(): string {
    const { min, max } = PROJECT_PREVIEW_PORT_RANGE;
    return `\\d{${String(min).length},${String(max).length}}`;
  }

  private isValidPort(port: string): boolean {
    const value = Number(port);
    return (
      Number.isInteger(value) &&
      value >= PROJECT_PREVIEW_PORT_RANGE.min &&
      value <= PROJECT_PREVIEW_PORT_RANGE.max
    );
  }

  private normalizeHostname(hostname: string): string {
    return hostname.trim().toLowerCase().replace(/\.$/, "");
  }

  private escapeRegExp(value: string): string {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }
}

export const projectPreviewUrlService = new ProjectPreviewUrlService();
