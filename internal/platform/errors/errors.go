// Package errors is the one place application errors are typed and mapped
// to HTTP semantics. Handlers never construct HTTP error bodies by hand —
// they return an *Error (or a plain error, which ToProblemDetails treats as
// internal) and the transport layer maps it, once, centrally
// (documentation/03-architecture-overview.md §7.4).
package errors

import "net/http"

// Kind is the closed set of application error categories. Each maps to
// exactly one HTTP status, so a handler that picks the right Kind never has
// to think about status codes directly.
type Kind string

const (
	KindNotFound     Kind = "not_found"
	KindConflict     Kind = "conflict"
	KindValidation   Kind = "validation"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindExternal     Kind = "external"
	KindInternal     Kind = "internal"
)

// StatusFor maps a Kind to its HTTP status code. This lives in
// platform/errors rather than the transport layer because the mapping is
// part of the error's definition, not a per-handler decision.
func StatusFor(k Kind) int {
	switch k {
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindValidation:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindExternal:
		return http.StatusBadGateway
	case KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// FieldError is one field-level validation failure, matching the `errors`
// array in the RFC 9457 body (documentation/07-api-specification.md §1.3).
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is the one typed application error every layer above the repository
// deals in. Code is the stable, namespaced, machine-readable identifier the
// frontend switches on (never Title or Detail, which are prose and may
// change). Err holds the real underlying cause for logging — it is never
// serialised to the client.
type Error struct {
	Kind   Kind
	Code   string // e.g. "project.not_found"
	Title  string
	Detail string
	Fields []FieldError
	Err    error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Detail + ": " + e.Err.Error()
	}
	return e.Detail
}

func (e *Error) Unwrap() error {
	return e.Err
}

// NotFound reports that a resource does not exist, or exists but belongs to
// another organisation — those two cases must be indistinguishable to the
// caller (documentation/07-api-specification.md §1.4, FR-IAM-008): a 403
// confirms the resource exists, which is itself an information leak.
func NotFound(code, detail string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Title: "Not found", Detail: detail}
}

// Conflict reports a state conflict — a duplicate name, a scan already
// running, a replayed refresh token.
func Conflict(code, detail string) *Error {
	return &Error{Kind: KindConflict, Code: code, Title: "Conflict", Detail: detail}
}

// Validation reports one or more invalid input fields.
func Validation(code, detail string, fields []FieldError) *Error {
	return &Error{Kind: KindValidation, Code: code, Title: "Validation failed", Detail: detail, Fields: fields}
}

// Unauthorized reports a missing or invalid credential (not logged in, or
// logged in with an expired/invalid token).
func Unauthorized(code, detail string) *Error {
	return &Error{Kind: KindUnauthorized, Code: code, Title: "Unauthorized", Detail: detail}
}

// Forbidden reports a role/permission failure on a resource the caller can
// otherwise see. Use NotFound instead when the caller should not even learn
// the resource exists.
func Forbidden(code, detail string) *Error {
	return &Error{Kind: KindForbidden, Code: code, Title: "Forbidden", Detail: detail}
}

// External reports a failure in a call to a system GuardPipe does not
// control (Gemini, OSV, GitHub, Docker). err is the real transport-level
// cause, kept for logging.
func External(code, detail string, err error) *Error {
	return &Error{Kind: KindExternal, Code: code, Title: "Upstream service error", Detail: detail, Err: err}
}

// Internal wraps an unexpected error. Its Detail is deliberately generic and
// never derived from err — internal error detail is logged, never returned
// to the client (documentation/03-architecture-overview.md §7.4).
func Internal(err error) *Error {
	return &Error{
		Kind:   KindInternal,
		Code:   "internal.unexpected",
		Title:  "Internal error",
		Detail: "An unexpected error occurred. If this persists, contact support with the request ID.",
		Err:    err,
	}
}
