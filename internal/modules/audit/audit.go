// Package audit is the cross-cutting append-only audit trail
// (documentation/06-database-design.md §4.19, DR-007). It isn't owned by
// any one engine or module — BUILD_GUIDE.md Phase 6 explicitly calls out
// wiring it into already-shipped Phase 2/3 code (modules/identity,
// modules/project), not just new Phase 6 code.
package audit

import (
	"context"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Entry is one audit_log row. OrgID/ActorID/ResourceType/ResourceID/IP are
// pointers because the table allows each to be null — a system action has
// no ActorID, a login has no ResourceID, and so on. ID/CreatedAt are unset
// on a write (Log doesn't need them, the insert is fire-and-forget) and
// populated on a read (List) — the same struct serves both directions
// rather than a second read-only type, since every other field is shared.
type Entry struct {
	ID           int64
	OrgID        *uuid.UUID
	ActorID      *uuid.UUID
	Action       string // e.g. "auth.login", "project.created", "target.attested"
	ResourceType *string
	ResourceID   *uuid.UUID
	Detail       map[string]any
	IP           *netip.Addr
	CreatedAt    time.Time
}

// ListFilter narrows List — every field is optional. OrgID is intentionally
// present: `GET /admin/audit-log` (BUILD_GUIDE.md Phase 14) is the one
// screen in the product where lifting the normal per-org scoping is the
// whole point, so a nil OrgID there means "every organisation," not a bug.
type ListFilter struct {
	OrgID   *uuid.UUID
	ActorID *uuid.UUID
	Action  *string
	From    *time.Time
	To      *time.Time
}

// Page is a 1-based page request, matching every other module's local copy
// of this shape (project.Page, admin.Page, ...).
type Page struct {
	Page     int
	PageSize int
}

// Repository is defined by this package; implementation lives in
// internal/store/repo. List is read-only and does not violate DR-007's
// "append-only, no UPDATE, no DELETE" — that rule is about mutating rows,
// not reading them; this interface still exposes no update/delete method.
type Repository interface {
	Insert(ctx context.Context, e Entry) error
	List(ctx context.Context, filter ListFilter, page Page) ([]Entry, int, error)
}

// Service records and reads back audit events. Log takes no error return
// and never fails the caller's own operation on a write failure — a
// database hiccup on the audit_log insert must not block a login or a
// project creation. The failure is logged instead, since audit-trail loss
// should be visible to an operator even though it isn't fatal to the
// request in flight.
type Service interface {
	Log(ctx context.Context, e Entry)
	List(ctx context.Context, filter ListFilter, page Page) ([]Entry, int, error)
}

type service struct {
	repo Repository
	log  *slog.Logger
}

func NewService(repo Repository, log *slog.Logger) Service {
	if log == nil {
		log = slog.Default()
	}
	return &service{repo: repo, log: log}
}

func (s *service) Log(ctx context.Context, e Entry) {
	if err := s.repo.Insert(ctx, e); err != nil {
		s.log.Error("audit: failed to record event", "action", e.Action, "error", err)
	}
}

func (s *service) List(ctx context.Context, filter ListFilter, page Page) ([]Entry, int, error) {
	return s.repo.List(ctx, filter, page)
}
