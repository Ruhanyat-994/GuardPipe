// Package validate wraps go-playground/validator/v10 behind a small API
// that returns plain-language field errors, so handlers can hand them
// straight to platform/errors.Validation without knowing anything about
// the validator library's own error types.
package validate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// FieldError is one field-level validation failure. It has the same shape
// as platform/errors.FieldError on purpose — callers convert directly
// between the two without a translation step.
type FieldError struct {
	Field   string
	Message string
}

// Validator validates structs using `validate:"..."` tags.
type Validator struct {
	v *validator.Validate
}

// New returns a Validator that reports each failed field by its `json` tag
// name rather than its Go field name, since request DTOs already declare
// that name and it's what the API contract and the frontend both use.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})
	return &Validator{v: v}
}

// Struct validates s against its `validate` tags. It returns nil if s is
// valid, or one FieldError per failed field otherwise — never the
// validator library's own error type, so this package is the only place
// that needs to know how to read it.
func (val *Validator) Struct(s any) []FieldError {
	err := val.v.Struct(s)
	if err == nil {
		return nil
	}

	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		// Programmer error (validating a non-struct) — not a user input
		// problem, so it does not belong in the returned field errors.
		return nil
	}

	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return nil
	}

	fieldErrs := make([]FieldError, 0, len(verrs))
	for _, fe := range verrs {
		fieldErrs = append(fieldErrs, FieldError{
			Field:   fe.Field(), // the json tag name, via RegisterTagNameFunc
			Message: message(fe),
		})
	}
	return fieldErrs
}

// message turns a small, known set of validator tags into a plain-language
// sentence (documentation/07-api-specification.md §1.3's example: "must be
// between 1 and 120 characters", not "Key: '...' Error:Field validation for
// '...' failed on the 'min' tag"). Tags outside this set fall back to a
// generic-but-still-plain message rather than leaking the tag name.
func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url":
		return "must be a valid URL"
	case "min":
		if fe.Kind().String() == "string" {
			return fmt.Sprintf("must be at least %s characters", fe.Param())
		}
		return fmt.Sprintf("must be at least %s", fe.Param())
	case "max":
		if fe.Kind().String() == "string" {
			return fmt.Sprintf("must be at most %s characters", fe.Param())
		}
		return fmt.Sprintf("must be at most %s", fe.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fe.Param())
	case "uuid", "uuid4":
		return "must be a valid UUID"
	default:
		return "is invalid"
	}
}
