/**
 * Chat deep links.
 *
 * The SPA has no URL router, so outbound notifications link to the application
 * root with a `?chat=<id>` query parameter. On boot the workspace reads that
 * parameter once, selects the chat, and strips it from the address bar so a
 * later manual chat switch does not leave a stale link behind.
 */

const CHAT_QUERY_PARAM = "chat";

/** Chat ids are lowercase hex, 4-32 chars (mirrors the backend's ValidID). */
const CHAT_ID_PATTERN = /^[0-9a-f]{4,32}$/;

class ChatDeepLinkState {
  /**
   * Reads the requested chat id from a location search string. Anything that
   * is not a well-formed chat id is ignored, so a crafted link cannot steer
   * the app into requesting arbitrary paths.
   */
  parse(search: string): string | null {
    if (!search) return null;
    const value = new URLSearchParams(search).get(CHAT_QUERY_PARAM);
    if (!value) return null;
    const candidate = value.trim().toLowerCase();
    return CHAT_ID_PATTERN.test(candidate) ? candidate : null;
  }

  /**
   * Returns the URL to replace the current one with once the deep link has
   * been consumed: the same page without the chat parameter.
   */
  withoutChatParam(pathname: string, search: string, hash: string): string {
    const params = new URLSearchParams(search);
    params.delete(CHAT_QUERY_PARAM);
    const query = params.toString();
    return `${pathname}${query ? `?${query}` : ""}${hash}`;
  }
}

export const chatDeepLinkState = new ChatDeepLinkState();
