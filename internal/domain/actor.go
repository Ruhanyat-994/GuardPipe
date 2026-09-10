package domain

import "github.com/google/uuid"

// Role matches the `user_role` Postgres enum (documentation/06-database-design.md
// §3). It is a shared-kernel type, not identity-owned, because every module
// checks it for authorisation — not just identity.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func (r Role) String() string {
	return string(r)
}

// Actor is the caller's identity as resolved from an access token, passed to
// every service method that takes a resource ID — resource-level
// authorisation (documentation/04-backend-architecture.md §11, "Authorisation
// — two layers") happens in the service, keyed off Actor, and cannot be
// skipped just because route-level RBAC already passed.
type Actor struct {
	UserID uuid.UUID
	OrgID  uuid.UUID
	Role   Role
	// ProjectID is nil for every ordinary org-scoped actor. It is set only
	// when the caller has switched into a single project shared with them
	// as an external collaborator (POST /auth/switch-project/{id}) — such a
	// session may act only on this one project (and whatever hangs off it —
	// scans, findings, documents, targets, reports), never anything else in
	// that project's organisation. Checked centrally in
	// project.service.getOwnedProject; every other project/scan/finding/
	// report authorisation check in the codebase funnels through it.
	ProjectID *uuid.UUID
}
