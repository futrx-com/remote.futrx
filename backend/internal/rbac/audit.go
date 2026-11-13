package rbac

// AuditOperation names the kind of policy mutation an AuditEvent records.
type AuditOperation string

const (
	AuditAssignmentSet     AuditOperation = "assignment.set"
	AuditAssignmentRemoved AuditOperation = "assignment.removed"
	AuditRoleCreated       AuditOperation = "role.created"
	AuditRoleUpdated       AuditOperation = "role.updated"
	AuditRoleDeleted       AuditOperation = "role.deleted"
	AuditRoleBound         AuditOperation = "binding.created"
	AuditRoleUnbound       AuditOperation = "binding.removed"
)

// SystemActorName is recorded as the actor of trusted internal mutations.
const SystemActorName = "system"

// AuditEvent is one durable record of a successful policy mutation. It never
// carries session material or unrelated user data.
type AuditEvent struct {
	At         int64
	Actor      string
	Operation  AuditOperation
	TargetUser string
	Permission Key
	RoleID     string
	Scope      Scope
	OldEffect  Effect
	NewEffect  Effect
	// Detail carries what the other fields cannot, such as a role's name and
	// rules.
	Detail string
}
