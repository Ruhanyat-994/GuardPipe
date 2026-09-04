// Package identity is authentication and access control, and nothing else
// (documentation/05-module-specifications.md §3). It knows nothing about
// projects or scans.
package identity

import (
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// User mirrors the `users` table (documentation/06-database-design.md §4.2).
// It is identity-owned, not a shared-kernel type — other modules that need
// to know "who did this" carry a domain.Actor (just OrgID/UserID/Role), not
// a full User.
type User struct {
	ID               uuid.UUID
	OrgID            uuid.UUID
	Email            string
	DisplayName      string
	PasswordHash     string
	Role             domain.Role
	LastLoginAt      *time.Time
	FailedLoginCount int
	LockedUntil      *time.Time
	// SuspendedAt/SuspendedReason (migration 00017, BUILD_GUIDE.md Phase 14)
	// are set only by modules/admin, through this module's own
	// UserRepository.SetSuspended — never by anything in this package
	// itself. identity only reads them, to reject a suspended account at
	// Login and on every subsequent authenticated request
	// (Service.CheckSuspension).
	SuspendedAt     *time.Time
	SuspendedReason *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RegisterInput is the input to Service.Register.
type RegisterInput struct {
	Email       string
	DisplayName string
	Password    string
}

// TokenPair is what a successful Login or Refresh returns. RefreshToken is
// the raw, one-time-visible value — only its SHA-256 hash is ever stored
// (documentation/06-database-design.md §4.3). User is populated on Login
// (the API response includes it) and left nil on Refresh (it doesn't).
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // seconds
	User         *User
}

// Claims is what Verify extracts from a valid access token.
type Claims struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      domain.Role
	JTI       string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// RefreshToken mirrors the `refresh_tokens` table (documentation/06-database-design.md
// §4.3). FamilyIssuedAt is copied unchanged onto every rotated token within
// a family — it's the original login time the whole chain traces back to,
// which is what lets Service.Refresh enforce an absolute session-lifetime
// cap (BUILD_GUIDE.md Phase 14) independent of how recently the session was
// used, without a second query per refresh.
type RefreshToken struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	TokenHash      string
	FamilyID       uuid.UUID
	CreatedAt      time.Time
	FamilyIssuedAt time.Time
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	RevokedAt      *time.Time
	UserAgent      *string
	IP             *string
	// OrgID (migration 00022, BUILD_GUIDE.md Phase 15) is the organisation
	// context this token pair was issued for — nil means "the user's home
	// organisation" (every ordinary login, and every token issued before
	// this column existed). Set explicitly by Service.IssueTokenPairForOrg
	// (POST /auth/switch-org) so Refresh rebuilds the next access token
	// against the org the caller actually switched to, not silently back
	// onto users.org_id.
	OrgID *uuid.UUID
}
