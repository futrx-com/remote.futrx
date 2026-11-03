import { useEffect, useState } from "preact/hooks";
import type { RbacAccountTarget, RbacScopeKind } from "../../../models/rbac";
import { usePermissionLookup } from "../../context/PermissionLookupContext";

export function usePermissionAccountTargets(kind: RbacScopeKind): RbacAccountTarget[] {
  const lookup = usePermissionLookup();
  const [accounts, setAccounts] = useState<RbacAccountTarget[]>([]);
  useEffect(() => {
    let disposed = false;
    if (kind === "provider-account") {
      void lookup.accountTargets().then((targets) => {
        if (!disposed) setAccounts(targets);
      });
    }
    return () => { disposed = true; };
  }, [kind, lookup]);
  return accounts;
}
