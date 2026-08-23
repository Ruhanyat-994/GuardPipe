package reporting_test

import (
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name string
		loc  domain.Location
		want string
	}{
		{
			name: "file with a line range",
			loc:  domain.Location{Type: domain.LocationTypeFile, Path: "src/app.go", LineStart: 10, LineEnd: 14},
			want: "src/app.go:10-14",
		},
		{
			name: "file with a single line",
			loc:  domain.Location{Type: domain.LocationTypeFile, Path: "src/app.go", LineStart: 10},
			want: "src/app.go:10",
		},
		{
			name: "file with no line at all",
			loc:  domain.Location{Type: domain.LocationTypeFile, Path: "README.md"},
			want: "README.md",
		},
		{
			name: "image layer",
			loc:  domain.Location{Type: domain.LocationTypeImage, Image: "myapp:latest", LayerDigest: "sha256:abcdef0123456789abcdef"},
			want: "myapp:latest (layer sha256:abcdef012345)",
		},
		{
			name: "network with a URL",
			loc:  domain.Location{Type: domain.LocationTypeNetwork, Host: "example.com", Port: 443, Protocol: "https", URL: "https://example.com/.git/HEAD"},
			want: "https://example.com/.git/HEAD",
		},
		{
			name: "network without a URL falls back to host:port/protocol",
			loc:  domain.Location{Type: domain.LocationTypeNetwork, Host: "example.com", Port: 443, Protocol: "HTTPS"},
			want: "example.com:443/https",
		},
		{
			name: "dependency",
			loc:  domain.Location{Type: domain.LocationTypeDependency, Ecosystem: "npm", Package: "left-pad", Version: "1.3.0", ManifestPath: "package.json"},
			want: "npm/left-pad@1.3.0 (package.json)",
		},
		{
			name: "k8s raw manifest",
			loc:  domain.Location{Type: domain.LocationTypeK8s, Kind: "Role", Name: "admin", Namespace: "default", FieldPath: "rules[0].verbs", File: "role.yaml"},
			want: "default/Role/admin (rules[0].verbs) in role.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reporting.FormatLocation(tt.loc); got != tt.want {
				t.Errorf("FormatLocation() = %q, want %q", got, tt.want)
			}
		})
	}
}
