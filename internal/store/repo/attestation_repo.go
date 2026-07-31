package repo

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
)

// AttestationRepo implements project.AttestationRepository against the
// append-only `target_attestations` table
// (documentation/06-database-design.md §4.8) — insert only, no update or
// delete method exists on this type by design.
type AttestationRepo struct {
	db Querier
}

func NewAttestationRepo(db Querier) *AttestationRepo {
	return &AttestationRepo{db: db}
}

var _ project.AttestationRepository = (*AttestationRepo)(nil)

func (r *AttestationRepo) Create(ctx context.Context, a *project.Attestation) error {
	sourceIP, err := netip.ParseAddr(a.SourceIP)
	if err != nil {
		return fmt.Errorf("repo: attestation source IP %q is not a valid address: %w", a.SourceIP, err)
	}

	const q = `
		INSERT INTO target_attestations (id, target_id, user_id, attestation_text_version, accepted_at, source_ip)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`
	if err := r.db.QueryRow(ctx, q, a.ID, a.TargetID, a.UserID, a.AttestationTextVersion, a.AcceptedAt, sourceIP).Scan(&a.ID); err != nil {
		return fmt.Errorf("repo: insert target attestation: %w", err)
	}
	return nil
}
