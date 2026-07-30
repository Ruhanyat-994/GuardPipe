package errors

import (
	"errors"
	"strings"
)

// problemBaseURL namespaces every error "type" URI. It does not need to
// resolve to a real page (RFC 9457 only requires it to be a stable
// identifier), but using the product's own domain keeps it unambiguous.
const problemBaseURL = "https://guardpipe.dev/errors/"

// ProblemDetails is the RFC 9457 response body every error maps to
// (documentation/07-api-specification.md §1.3). Instance and RequestID are
// filled in by the transport middleware, which is the only layer that knows
// the request path and the request ID.
type ProblemDetails struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"request_id,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// ToProblemDetails converts a typed Error into the wire format. instance is
// the request path (e.g. "/api/v1/projects"); requestID matches the log
// entry so a user-reported bug can be traced back to it.
func (e *Error) ToProblemDetails(instance, requestID string) ProblemDetails {
	return ProblemDetails{
		Type:      problemBaseURL + strings.ReplaceAll(string(e.Kind), "_", "-"),
		Title:     e.Title,
		Status:    StatusFor(e.Kind),
		Detail:    e.Detail,
		Instance:  instance,
		Code:      e.Code,
		RequestID: requestID,
		Errors:    e.Fields,
	}
}

// ToProblemDetails converts any error into the wire format. An error that is
// not a *Error (e.g. a bare error bubbling up from a dependency) is always
// treated as internal — this is what guarantees a handler can never leak
// unclassified error detail to a client, even if it forgets to wrap one.
func ToProblemDetails(err error, instance, requestID string) ProblemDetails {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.ToProblemDetails(instance, requestID)
	}
	return Internal(err).ToProblemDetails(instance, requestID)
}
