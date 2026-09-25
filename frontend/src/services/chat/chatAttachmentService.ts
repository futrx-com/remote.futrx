import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import type { Attachment } from "../../models/upload.ts";
import { CHAT_UPLOAD_PATHS } from "../../config/chat.ts";

const ATTACHED_FILES_HEADER = "Attached files:";
const ATTACHED_FILES_HEADER_LINE = `${ATTACHED_FILES_HEADER}\n`;
// Matches exactly the bullet shape `promptWithAttachments` writes: a leading
// dash followed by a single space. Tightening (vs. permissive whitespace)
// keeps the parser symmetrical with the writer.
const ATTACHED_BULLET_PREFIX = /^- /;

export interface AttachedFileSection {
  /** The user message with the `Attached files:` block stripped out. */
  message: string;
  /** The attachment paths in the order they were attached. */
  paths: string[];
}

class ChatAttachmentService {
  basePath(chat: ChatMeta, projects: ProjectMeta[]): string {
    const project = chat.projectId
      ? projects.find((candidate) => candidate.id === chat.projectId)
      : undefined;
    if (project) return this.uploadsUnder(CHAT_UPLOAD_PATHS.projectRoot);

    return this.uploadsUnder(this.normalizePath(chat.cwd || ""));
  }

  uniqueUploadName(name: string, token: string): string {
    const cleaned = name.split(/[\\/]/).pop()?.trim() || name.trim();
    if (!cleaned) return `file-${token}`;
    const dot = cleaned.lastIndexOf(".");
    if (dot <= 0) return `${cleaned}-${token}`;
    return `${cleaned.slice(0, dot)}-${token}${cleaned.slice(dot)}`;
  }

  absoluteUploadPath(basePath: string, fileName: string): string {
    const safeName = fileName.split(/[\\/]/).pop()?.trim() || fileName.trim();
    if (!safeName) return "";
    if (safeName.startsWith("/")) return safeName;

    const base = basePath.trim().replace(/\/+$/, "");
    if (!base) return safeName;
    return `${base}/${safeName}`;
  }

  // The prompt text that carries the uploaded paths to the agent.
  promptWithAttachments(userText: string, paths: string[]): string {
    if (paths.length === 0) return userText;
    const attachmentText = `${ATTACHED_FILES_HEADER}\n${paths.map((path) => `- ${path}`).join("\n")}`;
    return userText ? `${userText}\n\n${attachmentText}` : attachmentText;
  }

  /**
   * Parses the text produced by `promptWithAttachments` back into the original
   * user message and the attached file paths.
   *
   * The `Attached files:` block must occupy the entire tail of the message:
   * either preceded by `\n\n` (user text + attachments) or stand-alone at the
   * very start (attachment-only prompt). A header that appears mid-prose but is
   * not followed by bullet lines is left untouched, so the user's words render
   * verbatim.
   */
  parseAttachedPaths(text: string): AttachedFileSection {
    const unchanged = { message: text, paths: [] as string[] };
    if (!text) return { message: "", paths: [] };

    const headerOffset = text.indexOf(ATTACHED_FILES_HEADER_LINE);
    if (headerOffset < 0) return unchanged;

    // The block must be preceded by either the start of the text or a blank
    // line; otherwise we leave the user's prose untouched.
    const before = text.slice(0, headerOffset);
    if (headerOffset !== 0 && !before.endsWith("\n\n")) return unchanged;

    const body = text.slice(headerOffset + ATTACHED_FILES_HEADER_LINE.length);
    const lines = body.split("\n");

    const bulletLines: string[] = [];
    let i = 0;
    for (; i < lines.length; i++) {
      const line = lines[i];
      if (!line || !ATTACHED_BULLET_PREFIX.test(line)) break;
      bulletLines.push(line);
    }
    if (!bulletLines.length) return unchanged;

    // Reject the block if anything but blank lines trails the bullets.
    while (i < lines.length && lines[i] === "") i++;
    if (i < lines.length) return unchanged;

    const paths = bulletLines.map((line) => line.replace(ATTACHED_BULLET_PREFIX, "").trim());
    const message = before.endsWith("\n\n") ? before.slice(0, -2) : before;
    return { message, paths };
  }

  revokeObjectUrl(attachment: Attachment): void {
    if (attachment.objectUrl) URL.revokeObjectURL(attachment.objectUrl);
  }

  /** `<root>/.uploads`; an empty root yields the container-root default. */
  private uploadsUnder(root: string): string {
    return `${root}/${CHAT_UPLOAD_PATHS.dirName}`;
  }

  private normalizePath(path: string): string {
    const trimmed = path.trim();
    if (!trimmed) return "";
    return trimmed.replace(/\/+$/, "") || "/";
  }
}

export const chatAttachmentService = new ChatAttachmentService();
