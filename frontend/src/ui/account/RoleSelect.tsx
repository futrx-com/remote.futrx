import type { RbacRole } from "../../models/rbac";
import { ADMIN_ROLE_ID, USER_ROLES } from "../../config/constants/user-roles";
import {
  decodeRoleChoice,
  encodeRoleChoice,
  type RoleChoice,
} from "./roleChoice";

// Lists every base role and every custom role known to the system.
export function RoleSelect({
  value,
  customRoles,
  boundRoleIds,
  disabled,
  ariaLabel,
  title,
  class: className,
  onSelect,
}: {
  value: RoleChoice;
  customRoles: RbacRole[];
  boundRoleIds?: ReadonlySet<string>;
  disabled?: boolean;
  ariaLabel: string;
  title?: string;
  class: string;
  onSelect: (choice: RoleChoice) => void;
}) {
  const selectableRoles = customRoles.filter((role) => role.id !== ADMIN_ROLE_ID);
  return (
    <select
      value={encodeRoleChoice(value)}
      disabled={disabled}
      onChange={(e) =>
        onSelect(decodeRoleChoice((e.target as HTMLSelectElement).value))
      }
      aria-label={ariaLabel}
      title={title}
      class={className}
    >
      {value.kind === "base" && value.role === "" && (
        <option value={encodeRoleChoice(value)} disabled hidden>
          Select role
        </option>
      )}
      <optgroup label="Base roles">
        {USER_ROLES.map((role) => (
          <option key={role} value={encodeRoleChoice({ kind: "base", role })}>
            {role}
          </option>
        ))}
      </optgroup>
      {selectableRoles.length > 0 && (
        <optgroup label="Custom roles">
          {selectableRoles.map((role) => (
            <option
              key={role.id}
              value={encodeRoleChoice({ kind: "custom", roleId: role.id })}
              disabled={boundRoleIds?.has(role.id)}
            >
              {role.name}
              {boundRoleIds?.has(role.id) ? " ✓" : ""}
            </option>
          ))}
        </optgroup>
      )}
    </select>
  );
}
