import { useEffect, useState } from "preact/hooks";
import { emailApi } from "../../../api/emailApi";

/**
 * Whether the server can send email, read fresh each time the invite form
 * becomes relevant. Unknown or unreadable counts as "not configured": the
 * form then offers to hand the credentials over rather than promise a mail.
 */
export function useSmtpConfigured(enabled: boolean): boolean {
  const [configured, setConfigured] = useState(false);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    emailApi
      .get()
      .then((settings) => {
        if (!cancelled) setConfigured(settings.configured);
      })
      .catch(() => {
        if (!cancelled) setConfigured(false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  return configured;
}
