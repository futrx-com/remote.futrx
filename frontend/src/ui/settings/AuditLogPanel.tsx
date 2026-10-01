import { useAuditLog } from "../../state/hooks/admin/useAuditLog";
import { AuditLogSettings } from "./AuditLogSettings";
export function AuditLogPanel() {
 const audit = useAuditLog(true);
 return <AuditLogSettings {...audit} onFiltersChange={audit.setFilters} onRefresh={audit.refresh} onLoadMore={audit.loadMore} />;
}
