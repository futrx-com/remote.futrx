import { CHAT_TITLE_MAX_LENGTH } from "../../../config/chat.ts";

class ChatTitleState {
  /**
   * The title a rename should save, or null when there is nothing to save.
   * Whitespace runs collapse to one space, the way auto-titles are built, and
   * an empty result is refused: the server would store it, and the next prompt
   * would then silently replace it with an auto-title.
   */
  rename(input: string, current: string | undefined): string | null {
    const title = input.replace(/\s+/g, " ").trim().slice(0, CHAT_TITLE_MAX_LENGTH).trimEnd();
    if (!title || title === (current ?? "")) return null;
    return title;
  }
}

export const chatTitleState = new ChatTitleState();
