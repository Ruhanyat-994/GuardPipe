// Package dto is the wire format for every HTTP request/response. Domain
// and module types never appear directly in a response
// (documentation/04-backend-architecture.md §4.2) — this package is the
// translation layer, so an internal rename never breaks the frontend.
package dto

import (
	"time"

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
type UserResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

func FromUser(u *identity.User) UserResponse {
	return UserResponse{
		ID:          u.ID.String(),
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        string(u.Role),
		CreatedAt:   u.CreatedAt,
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
