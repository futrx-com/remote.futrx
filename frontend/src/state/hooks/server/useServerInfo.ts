import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { serverInfoApi } from "../../../api/serverInfoApi";
import { SERVER_INFO_REFRESH_INTERVAL_MS } from "../../../config/server";
import type { ServerInfo } from "../../../models/serverInfo";

export function useServerInfo(enabled: boolean) {
  const [info, setInfo] = useState<ServerInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestInFlight = useRef(false);
  const currentInfo = useRef<ServerInfo | null>(null);

  const refresh = useCallback(async () => {
    if (requestInFlight.current) return;
    requestInFlight.current = true;
    if (currentInfo.current == null) setLoading(true);
    else setRefreshing(true);
    try {
      const next = await serverInfoApi.fetch();
      currentInfo.current = next;
      setInfo(next);
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      requestInFlight.current = false;
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
    const interval = window.setInterval(() => void refresh(), SERVER_INFO_REFRESH_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [enabled, refresh]);

  return { info, loading, refreshing, error, refresh };
}
