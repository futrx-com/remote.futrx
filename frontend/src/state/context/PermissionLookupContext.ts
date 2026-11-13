import { createContext } from "preact";
import { useContext } from "preact/hooks";
import type { PermissionLookup } from "../../port/permissions";

export const PermissionLookupContext = createContext<PermissionLookup | null>(null);

export function usePermissionLookup(): PermissionLookup {
  const lookup = useContext(PermissionLookupContext);
  if (!lookup) throw new Error("Permission lookup requires PermissionLookupContext.Provider");
  return lookup;
}
