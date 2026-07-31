package identity

import (
	"fmt"
	"strings"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// minPasswordLength matches the policy in documentation/05-module-specifications.md
// §3 ("≥ 12 characters").
const minPasswordLength = 12

// commonPasswords is the "small common-password list" the same doc calls
// for — not exhaustive, just enough to catch the most obvious weak choices
// that happen to also be ≥ 12 characters (length alone lets through things
// like "password1234").
var commonPasswords = map[string]bool{
	"password1234":              true,
	"password12345":             true,
	"letmein123456":             true,
	"qwertyuiop123":             true,
	"123456789012":              true,
	"administrator":             true,
	"changeme12345":             true,
	"iloveyou12345":             true,
	"welcome123456":             true,
	"correcthorsebatterystaple": true, // the XKCD example itself — too well-known to count as random
}

// validatePassword enforces the password policy. It returns a
// *apperrors.Error directly (not a bare error) so a handler can pass it
// straight to c.Error without re-wrapping.
func validatePassword(password string) error {
	if len(password) < minPasswordLength {
		return apperrors.Validation(
			"identity.weak_password",
			fmt.Sprintf("password must be at least %d characters", minPasswordLength),
			[]apperrors.FieldError{{Field: "password", Message: fmt.Sprintf("must be at least %d characters", minPasswordLength)}},
		)
	}
	if commonPasswords[strings.ToLower(password)] {
		return apperrors.Validation(
			"identity.weak_password",
			"password is too common",
			[]apperrors.FieldError{{Field: "password", Message: "is too common — choose something less predictable"}},
		)
	}
	return nil
}
