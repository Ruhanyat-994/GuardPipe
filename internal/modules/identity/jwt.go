package identity

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// accessTokenClaims carries exactly the fields documented in
// documentation/05-module-specifications.md §3: "claims sub/org_id/role/jti/iat/exp".
type accessTokenClaims struct {
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

// TokenIssuer issues and verifies HS256 access tokens. HS256 (not RS256) is
// appropriate here because the same process both issues and verifies tokens
// — there's no separate service that only needs to verify, which is the
// usual reason to reach for asymmetric signing.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenIssuer builds a TokenIssuer. secret is GUARDPIPE_JWT_SECRET
// (already validated ≥ 32 bytes by platform/config); ttl is
// GUARDPIPE_ACCESS_TOKEN_TTL (15 min by default).
func NewTokenIssuer(secret []byte, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: secret, ttl: ttl}
}

// Issue mints a new access token for the given identity.
func (t *TokenIssuer) Issue(userID, orgID uuid.UUID, role domain.Role) (string, error) {
	now := time.Now().UTC()
	claims := accessTokenClaims{
		OrgID: orgID.String(),
		Role:  role.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
			ID:        id.New().String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("identity: sign access token: %w", err)
	}
	return signed, nil
}

// Parse verifies tokenString's signature and expiry and extracts Claims.
// The returned error wraps jwt.ErrTokenExpired specifically when the token
// is expired (checked with errors.Is), so the caller can return the
// documented `auth.token_expired` code instead of a generic
// `auth.token_invalid` (documentation/07-api-specification.md's error code
// catalogue — the SPA relies on this distinction to know whether to
// silently refresh or force a re-login).
func (t *TokenIssuer) Parse(tokenString string) (*Claims, error) {
	var claims accessTokenClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", tok.Header["alg"])
		}
		return t.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, jwt.ErrTokenSignatureInvalid
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("identity: malformed subject claim: %w", err)
	}
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		return nil, fmt.Errorf("identity: malformed org_id claim: %w", err)
	}

	return &Claims{
		UserID:    userID,
		OrgID:     orgID,
		Role:      domain.Role(claims.Role),
		JTI:       claims.ID,
		IssuedAt:  claims.IssuedAt.Time,
		ExpiresAt: claims.ExpiresAt.Time,
	}, nil
}
