import type { RbacDefinition } from "../../../models/rbac";
import { PERMISSIONS_EMPTY_COPY } from "../../../state/hooks/permissions/permissionsView";
import { Empty } from "../../projects/project-containers/ProjectContainerPrimitives";
import { PermissionBadge, PermissionsRow, PermissionsSection } from "./PermissionsPrimitives";

export function DefinitionsList({ definitions }: { definitions: RbacDefinition[] }) {
  return (
    <PermissionsSection
      title="Permission definitions"
      description="Permissions registered by the server. Read-only."
    >
      {definitions.length === 0 ? (
        <Empty text={PERMISSIONS_EMPTY_COPY.definitions} compact />
      ) : (
        definitions.map((definition) => (
          <PermissionsRow key={definition.key}>
            <span class="font-mono text-[12px] text-ink-50 truncate" title={definition.key}>
              {definition.key}
            </span>
            {definition.scopes.map((scope) => (
              <PermissionBadge key={scope} tone="neutral">{scope}</PermissionBadge>
            ))}
            <PermissionBadge tone="neutral">{`baseline: ${definition.baseline}`}</PermissionBadge>
            {definition.delegable && <PermissionBadge tone="highlight">delegable</PermissionBadge>}
            <div class="basis-full text-[11.5px] text-ink-300 leading-snug">
              {definition.description}
            </div>
          </PermissionsRow>
        ))
      )}
    </PermissionsSection>
  );
}
