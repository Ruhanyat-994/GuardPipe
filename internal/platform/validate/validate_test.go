package validate_test

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
)

type createProjectRequest struct {
	Name           string `json:"name" validate:"required,min=1,max=120"`
	RepositoryURL  string `json:"repository_url" validate:"required,url"`
	OwnerEmail     string `json:"owner_email" validate:"omitempty,email"`
	VisibilityMode string `json:"visibility_mode" validate:"required,oneof=public private"`
}

func TestStruct_ValidInputReturnsNoErrors(t *testing.T) {
	v := validate.New()
	req := createProjectRequest{
		Name:           "Payments API",
		RepositoryURL:  "https://github.com/acme/payments-api",
		OwnerEmail:     "nadia@example.com",
		VisibilityMode: "private",
	}

	if got := v.Struct(req); got != nil {
		t.Errorf("Struct() = %+v, want nil for valid input", got)
	}
}

func TestStruct_MissingRequiredFieldsReportsEachOne(t *testing.T) {
	v := validate.New()
	req := createProjectRequest{} // everything required is missing

	got := v.Struct(req)
	if len(got) == 0 {
		t.Fatal("Struct() returned no errors for an all-missing request")
	}

	fields := make(map[string]bool, len(got))
	for _, fe := range got {
		fields[fe.Field] = true
		if fe.Message == "" {
			t.Errorf("field %q has an empty message", fe.Field)
		}
	}
	for _, want := range []string{"name", "repository_url", "visibility_mode"} {
		if !fields[want] {
			t.Errorf("expected a field error for %q, got %+v", want, got)
		}
	}
	// owner_email is optional (omitempty) — a near-miss check that it must
	// NOT be reported just because it's blank.
	if fields["owner_email"] {
		t.Errorf("owner_email is optional and blank; it must not be reported as invalid")
	}
}

func TestStruct_FieldNamesUseJSONTagNotGoFieldName(t *testing.T) {
	v := validate.New()
	got := v.Struct(createProjectRequest{Name: "x", RepositoryURL: "https://a", VisibilityMode: "bogus"})

	for _, fe := range got {
		if fe.Field == "VisibilityMode" {
			t.Errorf("field name leaked the Go struct field name %q instead of the json tag", fe.Field)
		}
	}
}

func TestStruct_InvalidURLIsRejected(t *testing.T) {
	v := validate.New()
	got := v.Struct(createProjectRequest{
		Name: "x", RepositoryURL: "not-a-url", VisibilityMode: "public",
	})

	found := false
	for _, fe := range got {
		if fe.Field == "repository_url" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected repository_url to be reported invalid for %q", "not-a-url")
	}
}

// TestStruct_ValidURLIsNotRejected is the near-miss half of the previous
// test: a well-formed URL must not be flagged just because the field is
// also required.
func TestStruct_ValidURLIsNotRejected(t *testing.T) {
	v := validate.New()
	got := v.Struct(createProjectRequest{
		Name: "x", RepositoryURL: "https://github.com/acme/repo", VisibilityMode: "public",
	})
	for _, fe := range got {
		if fe.Field == "repository_url" {
			t.Errorf("a valid HTTPS URL was rejected: %+v", fe)
		}
	}
}

func TestStruct_OneofRejectsValueOutsideTheSet(t *testing.T) {
	v := validate.New()
	got := v.Struct(createProjectRequest{
		Name: "x", RepositoryURL: "https://a.com", VisibilityMode: "super-secret",
	})
	found := false
	for _, fe := range got {
		if fe.Field == "visibility_mode" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected visibility_mode to be reported invalid for a value outside oneof")
	}
}
