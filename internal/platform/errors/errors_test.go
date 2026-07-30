package errors_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

func TestStatusFor(t *testing.T) {
	tests := []struct {
		kind apperrors.Kind
		want int
	}{
		{apperrors.KindNotFound, http.StatusNotFound},
		{apperrors.KindConflict, http.StatusConflict},
		{apperrors.KindValidation, http.StatusBadRequest},
		{apperrors.KindUnauthorized, http.StatusUnauthorized},
		{apperrors.KindForbidden, http.StatusForbidden},
		{apperrors.KindExternal, http.StatusBadGateway},
		{apperrors.KindInternal, http.StatusInternalServerError},
		{apperrors.Kind("made_up"), http.StatusInternalServerError}, // near-miss: unknown kind fails safe, not open
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := apperrors.StatusFor(tt.kind); got != tt.want {
				t.Errorf("StatusFor(%q) = %d, want %d", tt.kind, got, tt.want)
			}
		})
	}
}

func TestConstructors_SetKindAndCode(t *testing.T) {
	tests := []struct {
		name     string
		err      *apperrors.Error
		wantKind apperrors.Kind
	}{
		{"NotFound", apperrors.NotFound("project.not_found", "no such project"), apperrors.KindNotFound},
		{"Conflict", apperrors.Conflict("project.name_taken", "duplicate name"), apperrors.KindConflict},
		{"Unauthorized", apperrors.Unauthorized("auth.invalid_credentials", "bad password"), apperrors.KindUnauthorized},
		{"Forbidden", apperrors.Forbidden("auth.forbidden_role", "role lacks permission"), apperrors.KindForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", tt.err.Kind, tt.wantKind)
			}
			if tt.err.Code == "" {
				t.Errorf("Code must not be empty")
			}
		})
	}
}

func TestValidation_CarriesFieldErrors(t *testing.T) {
	fields := []apperrors.FieldError{
		{Field: "name", Message: "must be between 1 and 120 characters"},
	}
	err := apperrors.Validation("project.validation_failed", "One or more fields are invalid.", fields)

	if len(err.Fields) != 1 || err.Fields[0].Field != "name" {
		t.Errorf("Fields = %+v, want the single 'name' field error", err.Fields)
	}
}

func TestExternal_WrapsUnderlyingError(t *testing.T) {
	cause := errors.New("connection refused")
	err := apperrors.External("ai.unavailable", "Gemini unreachable", cause)

	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false, want true — External must wrap its cause for errors.Is/As to work")
	}
}

// TestInternal_NeverLeaksCauseInDetail is the security-relevant near-miss
// case: platform/errors exists specifically so internal error detail is
// logged but never returned to the client
// (documentation/03-architecture-overview.md §7.4). A regression here would
// leak stack-trace-adjacent detail (SQL, file paths, driver errors) straight
// to an attacker probing the API.
func TestInternal_NeverLeaksCauseInDetail(t *testing.T) {
	cause := errors.New("pq: password authentication failed for user \"guardpipe\" at 10.0.0.5:5432")
	err := apperrors.Internal(cause)

	if strings.Contains(err.Detail, "password") || strings.Contains(err.Detail, "10.0.0.5") {
		t.Fatalf("Internal().Detail leaked the underlying cause: %q", err.Detail)
	}
	// The cause must still be reachable for logging via Unwrap/errors.Is.
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false, want true — the cause must still be loggeable via Unwrap")
	}
}

func TestToProblemDetails_TypedError(t *testing.T) {
	err := apperrors.Validation("project.validation_failed", "One or more fields are invalid.",
		[]apperrors.FieldError{{Field: "name", Message: "required"}})

	pd := apperrors.ToProblemDetails(err, "/api/v1/projects", "req-123")

	if pd.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", pd.Status, http.StatusBadRequest)
	}
	if pd.Code != "project.validation_failed" {
		t.Errorf("Code = %q, want %q", pd.Code, "project.validation_failed")
	}
	if pd.Instance != "/api/v1/projects" || pd.RequestID != "req-123" {
		t.Errorf("Instance/RequestID = %q/%q, want the values passed in", pd.Instance, pd.RequestID)
	}
	if len(pd.Errors) != 1 {
		t.Errorf("Errors = %+v, want the one field error", pd.Errors)
	}
}

// TestToProblemDetails_BareErrorIsTreatedAsInternal is the near-miss that
// matters most: a handler that forgets to wrap a plain error in *Error must
// still fail safe to a generic 500, never a raw error string reaching the
// client.
func TestToProblemDetails_BareErrorIsTreatedAsInternal(t *testing.T) {
	bare := errors.New("dial tcp 10.0.0.5:5432: connect: connection refused")

	pd := apperrors.ToProblemDetails(bare, "/api/v1/scans", "req-456")

	if pd.Status != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d for an unwrapped error", pd.Status, http.StatusInternalServerError)
	}
	if strings.Contains(pd.Detail, "10.0.0.5") {
		t.Fatalf("Detail leaked the bare error's message: %q", pd.Detail)
	}
}
