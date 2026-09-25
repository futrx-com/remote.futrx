import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import type { Attachment } from "../../models/upload.ts";
import { CHAT_UPLOAD_PATHS } from "../../config/chat.ts";

const ATTACHED_FILES_HEADER = "Attached files:";

// Single bullet prefix used in the rendered prompt. Files whose names start
// with the prefix char survive the parser; we therefore slice off only the
// first two chars ("- ").
const ATTACHED_BULLET_PREFIX = /^\s*-\s?/;

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
    if (!text) return { message: "", paths: [] };
    const headerOffset = text.indexOf(`${ATTACHED_FILES_HEADER}\n`);
    if (headerOffset < 0) return { message: text, paths: [] };

    // Must begin at the start or be preceded by a blank line.
    const headerStart = headerOffset + `${ATTACHED_FILES_HEADER}\n`.length;
    if (headerOffset !== 0) {
      const before = text.slice(0, headerOffset);
      if (!before.endsWith("\n\n")) return { message: text, paths: [] };
    }

    const body = text.slice(headerStart);
    const lines = body.split("\n");
    if (!lines.length || !ATTACHED_BULLET_PREFIX.test(lines[0])) {
      return { message: text, paths: [] };
    }

    const bulletLines: string[] = [];
    let i = 0;
    for (; i < lines.length; i++) {
      if (!lines[i] || !ATTACHED_BULLET_PREFIX.test(lines[i])) break;
      bulletLines.push(lines[i]);
    }
    if (!bulletLines.length) return { message: text, paths: [] };

    // Reject the block if any non-blank line after the bullets is not part of
    // the trailing message tail. The bullet block extends to the first
    // non-bullet, non-blank line.
    while (i < lines.length && lines[i] === "") i++;
    if (i < lines.length) {
      return { message: text, paths: [] };
    }

    const paths = bulletLines
      .map((line) => line.replace(ATTACHED_BULLET_PREFIX, "").trim())
      .filter((line) => line.length > 0);
    if (!paths.length) return { message: text, paths: [] };

    const before = text.slice(0, headerOffset);
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
