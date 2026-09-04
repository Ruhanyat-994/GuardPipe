// Package dto is the wire format for every HTTP request/response. Domain
// and module types never appear directly in a response
// (documentation/04-backend-architecture.md §4.2) — this package is the
// translation layer, so an internal rename never breaks the frontend.
package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
)

// RegisterRequest matches documentation/07-api-specification.md §2,
// `POST /auth/register`.
type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email"`
	DisplayName string `json:"display_name" validate:"required,min=1,max=120"`
	Password    string `json:"password" validate:"required,min=12"`
}

func (r RegisterRequest) ToInput() identity.RegisterInput {
	return identity.RegisterInput{Email: r.Email, DisplayName: r.DisplayName, Password: r.Password}
}

// UserResponse matches the user shape in documentation/07-api-specification.md
// §2 — never includes PasswordHash or any other internal field.
// IsPlatformOperator (BUILD_GUIDE.md Phase 14) is the one addition since:
// it's what the SPA's route guard reads to decide whether to render the
// `/admin` nav entry and route tree at all — a non-operator shouldn't even
// see it exists, not just get a 403 clicking it. The real enforcement is
// still server-side (RequirePlatformOperator); this field is UX only.
// OrgID/Role (BUILD_GUIDE.md Phase 15) are the caller's *active* org
// context — actor.OrgID/actor.Role from the JWT claims, not necessarily
// identity.User's own OrgID/Role (the account's home org) once switch-org
// exists: a switched session's `/auth/me` must report the org it's
// currently acting as, not silently fall back to home. Login/Register still
// pass the freshly-created/home values here, which are identical to the
// user's own fields at that point anyway.
type UserResponse struct {
	ID                 string    `json:"id"`
	Email              string    `json:"email"`
	DisplayName        string    `json:"display_name"`
	OrgID              string    `json:"org_id"`
	Role               string    `json:"role"`
	IsPlatformOperator bool      `json:"is_platform_operator"`
	CreatedAt          time.Time `json:"created_at"`
}

func FromUser(u *identity.User, isPlatformOperator bool) UserResponse {
	return FromUserInOrg(u, u.OrgID, u.Role, isPlatformOperator)
}

// FromUserInOrg is FromUser with an explicit active-org override — see the
// type's own doc comment. Me (`GET /auth/me`) uses this with actor.OrgID/
// actor.Role; every other caller (Register, Login) uses FromUser, since
// there's no other org context to consider at those two moments.
func FromUserInOrg(u *identity.User, orgID uuid.UUID, role domain.Role, isPlatformOperator bool) UserResponse {
	return UserResponse{
		ID:                 u.ID.String(),
		Email:              u.Email,
		DisplayName:        u.DisplayName,
		OrgID:              orgID.String(),
		Role:               string(role),
		IsPlatformOperator: isPlatformOperator,
		CreatedAt:          u.CreatedAt,
	}
}

// LoginRequest matches `POST /auth/login`.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse matches documentation/07-api-specification.md §2's example
// exactly: access_token/token_type/expires_in/user.
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int          `json:"expires_in"`
	User        UserResponse `json:"user"`
}

// RefreshResponse — the refresh endpoint doesn't repeat the user object
// (documentation/07-api-specification.md §2's sequence diagram only shows
// `{access_token}`).
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}
