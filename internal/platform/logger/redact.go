package logger

import "strings"

// redactedKeys are attribute keys whose value is always replaced, whatever
// it contains — a caller that logs `slog.String("password", p)` should not
// need a matching secret-shaped value for it to be caught.
var redactedKeys = map[string]bool{
	"password":       true,
	"secret":         true,
	"token":          true,
	"access_token":   true,
	"refresh_token":  true,
	"api_key":        true,
	"apikey":         true,
	"authorization":  true,
	"jwt":            true,
	"jwt_secret":     true,
	"private_key":    true,
	"encryption_key": true,
	"client_secret":  true,
	"gemini_api_key": true,
}

const redactedPlaceholder = "[REDACTED]"

// isRedactedKey reports whether key names a value that must never reach the
// log, regardless of what it looks like.
func isRedactedKey(key string) bool {
	return redactedKeys[strings.ToLower(key)]
}
