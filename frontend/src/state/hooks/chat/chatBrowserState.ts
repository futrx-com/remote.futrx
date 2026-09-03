import { PUBLIC_HOSTNAME } from "../../../config/runtime.ts";
import type { BrowserElementCapture } from "../../../models/browser";
import type { ChatMessageBlock } from "../../../models/chatMessage";
import { projectPreviewUrlService } from "../../../services/projects/projectPreviewUrlService.ts";

class ChatBrowserState {
  latestPublicDevUrl(blocks: ChatMessageBlock[], projectSlug: string): string {
    let latest = "";
    for (const block of blocks) {
      for (const text of this.blockTexts(block)) {
        for (const candidate of projectPreviewUrlService.findInText(text, PUBLIC_HOSTNAME)) {
          if (projectPreviewUrlService.belongsToProject(candidate, projectSlug, PUBLIC_HOSTNAME)) latest = candidate;
        }
      }
    }
    return latest;
  }

  formatElementCapture(capture: BrowserElementCapture): string {
    const lines = ["[Browser element]", `URL: ${capture.url || ""}`];
    if (capture.title) lines.push(`Title: ${capture.title}`);
    lines.push(`Selector: ${capture.selector || ""}`);
    lines.push(`Tag: ${capture.tag || ""}`);
    if (capture.id) lines.push(`ID: ${capture.id}`);
    if (capture.classes?.length) lines.push(`Classes: ${capture.classes.join(" ")}`);
    if (capture.role) lines.push(`Role: ${capture.role}`);
    if (capture.ariaLabel) lines.push(`ARIA label: ${capture.ariaLabel}`);
    if (capture.rect) {
      lines.push(
        `Box: x=${capture.rect.x} y=${capture.rect.y} w=${capture.rect.width} h=${capture.rect.height}`
      );
    }
    if (capture.viewport) {
      lines.push(`Viewport: ${capture.viewport.width}x${capture.viewport.height}`);
    }
    if (capture.parents?.length) lines.push(`Parents: ${capture.parents.join(" > ")}`);
    if (capture.styles && Object.keys(capture.styles).length) {
      lines.push("Styles:");
      for (const [key, value] of Object.entries(capture.styles)) {
        if (value) lines.push(`- ${key}: ${value}`);
      }
    }
    if (capture.text) lines.push(`Text: ${capture.text}`);
    if (capture.html) lines.push(`HTML: ${capture.html}`);
    lines.push("[/Browser element]");
    return lines.join("\n");
  }

  private blockTexts(block: ChatMessageBlock): string[] {
    if (block.type === "user") return [block.text];
    if (block.type === "error") return [block.message];
    return block.parts.flatMap((part) => {
      if (part.kind === "text" || part.kind === "thinking") return [part.text];
      return part.output ? [part.output] : [];
    });
  }
}

export const chatBrowserState = new ChatBrowserState();
