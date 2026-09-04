import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import type { Attachment } from "../../models/upload.ts";
import { CHAT_UPLOAD_PATHS } from "../../config/chat.ts";

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
    const attachmentText = `Attached files:\n${paths.map((path) => `- ${path}`).join("\n")}`;
    return userText ? `${userText}\n\n${attachmentText}` : attachmentText;
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
