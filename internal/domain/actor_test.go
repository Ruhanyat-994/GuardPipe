package domain_test

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func TestRole_Valid(t *testing.T) {
	valid := []domain.Role{domain.RoleAdmin, domain.RoleMember, domain.RoleViewer}
	for _, r := range valid {
		if !r.Valid() {
			t.Errorf("Role(%q).Valid() = false, want true", r)
		}
	}
	if domain.Role("owner").Valid() {
		t.Errorf("near-miss Role value must not validate")
	}
}
