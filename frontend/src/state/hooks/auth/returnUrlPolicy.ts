import { MAX_RETURN_URL_LENGTH } from "../../../config/auth.ts";

class ReturnUrlPolicy {
  safeTarget(rawUrl: string, baseUrl: string): string {
    if (!rawUrl || rawUrl.length > MAX_RETURN_URL_LENGTH) return "";

    try {
      const target = new URL(rawUrl);
      const base = new URL(baseUrl);
      const allowedHost = target.host === base.host || target.host.endsWith(`.${base.host}`);
      return target.protocol === "https:" && allowedHost ? rawUrl : "";
    } catch {
      return "";
    }
  }
}

export const returnUrlPolicy = new ReturnUrlPolicy();
