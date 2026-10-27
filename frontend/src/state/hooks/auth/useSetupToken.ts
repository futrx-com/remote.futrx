import { useEffect, useState } from "preact/hooks";
import { setupTokenPolicy } from "./setupTokenPolicy";

// Owns the browser lifecycle of the first-boot setup token: capture it once,
// then remove it from the address bar while retaining the in-memory value for
// the eventual claim request.
export function useSetupToken(): string {
  const [setupToken] = useState(() => setupTokenPolicy.read(location.search));

  useEffect(() => {
    if (!setupToken) return;
    history.replaceState(
      null,
      "",
      setupTokenPolicy.strippedUrl(location.pathname, location.search),
    );
  }, [setupToken]);

  return setupToken;
}
