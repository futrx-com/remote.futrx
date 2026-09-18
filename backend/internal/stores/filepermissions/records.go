package filepermissions

import (
	"github.com/futrx-com/remote.futrx.com/internal/service/permission"
)

// schemaVersion is stored from the first release so a later layout change can
// migrate deliberately instead of guessing.
const schemaVersion = 1

type scopeRecord struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

type assignmentRecord struct {
	ID         string      `json:"id"`
	UserEmail  string      `json:"userEmail"`
	Permission string      `json:"permission"`
	Effect     string      `json:"effect"`
	Scope      scopeRecord `json:"scope"`
	CreatedBy  string      `json:"createdBy"`
	CreatedAt  int64       `json:"createdAt"`
}

type ruleRecord struct {
	Permission string `json:"permission"`
	Effect     string `json:"effect"`
}

type roleRecord struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Rules       []ruleRecord `json:"rules"`
	CreatedBy   string       `json:"createdBy"`
	CreatedAt   int64        `json:"createdAt"`
	UpdatedAt   int64        `json:"updatedAt"`
}

type bindingRecord struct {
	ID        string      `json:"id"`
	RoleID    string      `json:"roleId"`
	UserEmail string      `json:"userEmail"`
	Scope     scopeRecord `json:"scope"`
	CreatedBy string      `json:"createdBy"`
	CreatedAt int64       `json:"createdAt"`
}

// fileRecord is the on-disk shape of permissions.json.
type fileRecord struct {
	Version     int                `json:"version"`
	Assignments []assignmentRecord `json:"assignments"`
	Roles       []roleRecord       `json:"roles"`
	Bindings    []bindingRecord    `json:"bindings"`
}

// auditRecord is one line of permission-audit.jsonl.
type auditRecord struct {
	At         int64       `json:"at"`
	Actor      string      `json:"actor"`
	Operation  string      `json:"operation"`
	TargetUser string      `json:"targetUser,omitempty"`
	Permission string      `json:"permission,omitempty"`
	RoleID     string      `json:"roleId,omitempty"`
	Scope      scopeRecord `json:"scope"`
	OldEffect  string      `json:"oldEffect,omitempty"`
	NewEffect  string      `json:"newEffect,omitempty"`
	Detail     string      `json:"detail,omitempty"`
}

func scopeToRecord(scope permission.Scope) scopeRecord {
	return scopeRecord{Kind: string(scope.Kind), ID: scope.ID}
}

func scopeFromRecord(record scopeRecord) permission.Scope {
	return permission.Scope{Kind: permission.ScopeKind(record.Kind), ID: record.ID}
}

func recordFromState(state permission.State) fileRecord {
	out := fileRecord{
		Version:     schemaVersion,
		Assignments: make([]assignmentRecord, 0, len(state.Assignments)),
		Roles:       make([]roleRecord, 0, len(state.Roles)),
		Bindings:    make([]bindingRecord, 0, len(state.Bindings)),
	}
	for _, a := range state.Assignments {
		out.Assignments = append(out.Assignments, assignmentRecord{
			ID: a.ID, UserEmail: a.UserEmail, Permission: string(a.Permission), Effect: string(a.Effect),
			Scope: scopeToRecord(a.Scope), CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
		})
	}
	for _, role := range state.Roles {
		rules := make([]ruleRecord, 0, len(role.Rules))
		for _, rule := range role.Rules {
			rules = append(rules, ruleRecord{Permission: string(rule.Permission), Effect: string(rule.Effect)})
		}
		out.Roles = append(out.Roles, roleRecord{
			ID: role.ID, Name: role.Name, Description: role.Description, Rules: rules,
			CreatedBy: role.CreatedBy, CreatedAt: role.CreatedAt, UpdatedAt: role.UpdatedAt,
		})
	}
	for _, b := range state.Bindings {
		out.Bindings = append(out.Bindings, bindingRecord{
			ID: b.ID, RoleID: b.RoleID, UserEmail: b.UserEmail, Scope: scopeToRecord(b.Scope),
			CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt,
		})
	}
	return out
}

// stateFromRecord converts a parsed file to domain state, normalizing stored
// emails so a hand-edited file cannot disagree with lookups on case.
func stateFromRecord(record fileRecord) permission.State {
	var state permission.State
	for _, a := range record.Assignments {
		state.Assignments = append(state.Assignments, permission.Assignment{
			ID: a.ID, UserEmail: permission.NormalizeEmail(a.UserEmail),
			Permission: permission.Key(a.Permission), Effect: permission.Effect(a.Effect),
			Scope: scopeFromRecord(a.Scope), CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
		})
	}
	for _, role := range record.Roles {
		converted := permission.Role{
			ID: role.ID, Name: role.Name, Description: role.Description,
			CreatedBy: role.CreatedBy, CreatedAt: role.CreatedAt, UpdatedAt: role.UpdatedAt,
		}
		for _, rule := range role.Rules {
			converted.Rules = append(converted.Rules, permission.RoleRule{
				Permission: permission.Key(rule.Permission), Effect: permission.Effect(rule.Effect),
			})
		}
		state.Roles = append(state.Roles, converted)
	}
	for _, b := range record.Bindings {
		state.Bindings = append(state.Bindings, permission.RoleBinding{
			ID: b.ID, RoleID: b.RoleID, UserEmail: permission.NormalizeEmail(b.UserEmail),
			Scope: scopeFromRecord(b.Scope), CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt,
		})
	}
	return state
}

func auditRecordFromEvent(event permission.AuditEvent) auditRecord {
	return auditRecord{
		At: event.At, Actor: event.Actor, Operation: string(event.Operation),
		TargetUser: event.TargetUser, Permission: string(event.Permission), RoleID: event.RoleID,
		Scope: scopeToRecord(event.Scope), OldEffect: string(event.OldEffect),
		NewEffect: string(event.NewEffect), Detail: event.Detail,
	}
}
