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

	"github.com/google/uuid"
)

// Entry is one audit_log row. OrgID/ActorID/ResourceType/ResourceID/IP are
// pointers because the table allows each to be null — a system action has
// no ActorID, a login has no ResourceID, and so on.
type Entry struct {
	OrgID        *uuid.UUID
	ActorID      *uuid.UUID
	Action       string // e.g. "auth.login", "project.created", "target.attested"
	ResourceType *string
	ResourceID   *uuid.UUID
	Detail       map[string]any
	IP           *netip.Addr
}

// Repository is defined by this package; implementation lives in
// internal/store/repo. Insert-only — DR-007's "append-only, no UPDATE, no
// DELETE" is enforced by this interface never exposing anything else.
type Repository interface {
	Insert(ctx context.Context, e Entry) error
}

// Service records audit events. Log takes no error return and never fails
// the caller's own operation on a write failure — a database hiccup on the
// audit_log insert must not block a login or a project creation. The
// failure is logged instead, since audit-trail loss should be visible to
// an operator even though it isn't fatal to the request in flight.
type Service interface {
	Log(ctx context.Context, e Entry)
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
