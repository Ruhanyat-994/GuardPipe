package github_test

import (
	"errors"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{name: "plain https URL", raw: "https://github.com/acme/payments-api", wantOwner: "acme", wantRepo: "payments-api"},
		{name: "trailing .git is stripped", raw: "https://github.com/acme/payments-api.git", wantOwner: "acme", wantRepo: "payments-api"},
		{name: "trailing slash is tolerated", raw: "https://github.com/acme/payments-api/", wantOwner: "acme", wantRepo: "payments-api"},
		{name: "www. host is tolerated", raw: "https://www.github.com/acme/payments-api", wantOwner: "acme", wantRepo: "payments-api"},
		// Near-misses: each must fail, not silently accept a lookalike.
		{name: "http (not https) is rejected", raw: "http://github.com/acme/payments-api", wantErr: true},
		{name: "non-GitHub host is rejected", raw: "https://gitlab.com/acme/payments-api", wantErr: true},
		{name: "extra path segments are rejected", raw: "https://github.com/acme/payments-api/tree/main", wantErr: true},
		{name: "missing repo name is rejected", raw: "https://github.com/acme", wantErr: true},
		{name: "not a URL at all is rejected", raw: "not a url", wantErr: true},
		{name: "empty string is rejected", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := github.ParseRepoURL(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, github.ErrInvalidRepositoryURL) {
					t.Fatalf("ParseRepoURL(%q) error = %v, want ErrInvalidRepositoryURL", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepoURL(%q) unexpected error: %v", tt.raw, err)
			}
			if ref.Owner != tt.wantOwner || ref.Name != tt.wantRepo {
				t.Errorf("ParseRepoURL(%q) = %+v, want owner=%q name=%q", tt.raw, ref, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}
