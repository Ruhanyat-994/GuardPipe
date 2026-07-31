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
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
// §4.3).
type RefreshToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	FamilyID   uuid.UUID
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	RevokedAt  *time.Time
	UserAgent  *string
	IP         *string
}
