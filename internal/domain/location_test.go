package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// These golden cases are copied from the `location` JSONB contract examples
// in documentation/06-database-design.md §5. If a Location no longer
// marshals to this exact shape, the store and the frontend silently drift
// apart from the documented contract.
func TestLocation_MarshalJSON_MatchesDocumentedContract(t *testing.T) {
	tests := []struct {
		name string
		loc  domain.Location
		want string
	}{
		{
			name: "file location",
			loc: domain.Location{
				Type: domain.LocationTypeFile, Path: "src/db/user.go",
				LineStart: 42, LineEnd: 44, Column: 17,
			},
			want: `{"type":"file","path":"src/db/user.go","line_start":42,"line_end":44,"column":17}`,
		},
		{
			name: "image location",
			loc: domain.Location{
				Type: domain.LocationTypeImage, Image: "myapp:1.2.3",
				LayerDigest: "sha256:ab12", LayerIndex: 4, Path: "/usr/lib/libssl.so.1.1",
			},
			want: `{"type":"image","path":"/usr/lib/libssl.so.1.1","image":"myapp:1.2.3","layer_digest":"sha256:ab12","layer_index":4}`,
		},
		{
			name: "k8s location",
			loc: domain.Location{
				Type: domain.LocationTypeK8s, File: "deploy/api.yaml", Kind: "Deployment",
				Name: "api", Namespace: "prod",
				FieldPath: "spec.template.spec.containers[0].securityContext.privileged",
			},
			want: `{"type":"k8s","file":"deploy/api.yaml","kind":"Deployment","name":"api","namespace":"prod","field_path":"spec.template.spec.containers[0].securityContext.privileged"}`,
		},
		{
			name: "network location",
			loc: domain.Location{
				Type: domain.LocationTypeNetwork, Host: "example.com", IP: "203.0.113.10",
				Port: 443, Protocol: "tcp", Service: "https", URL: "https://example.com/admin",
			},
			want: `{"type":"network","host":"example.com","ip":"203.0.113.10","port":443,"protocol":"tcp","service":"https","url":"https://example.com/admin"}`,
		},
		{
			name: "dependency location",
			loc: domain.Location{
				Type: domain.LocationTypeDependency, Ecosystem: "npm", Package: "lodash",
				Version: "4.17.15", ManifestPath: "package.json",
			},
			want: `{"type":"dependency","ecosystem":"npm","package":"lodash","version":"4.17.15","manifest_path":"package.json"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.loc)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() =\n  %s\nwant\n  %s", got, tt.want)
			}

			// Round-trip: unmarshalling the marshalled form must reproduce
			// the original value, so the contract is symmetric.
			var roundTrip domain.Location
			if err := json.Unmarshal(got, &roundTrip); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if roundTrip != tt.loc {
				t.Errorf("round-trip = %+v, want %+v", roundTrip, tt.loc)
			}
		})
	}
}

func TestLocation_OmitsFieldsFromOtherShapes(t *testing.T) {
	// A near-miss check: a file location must not leak k8s/network/dependency
	// fields into its JSON just because the struct has them.
	loc := domain.Location{Type: domain.LocationTypeFile, Path: "a.go", LineStart: 1}
	got, err := json.Marshal(loc)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, unwanted := range []string{"kind", "namespace", "host", "ecosystem", "layer_digest"} {
		if strings.Contains(string(got), unwanted) {
			t.Errorf("file location JSON unexpectedly contains %q: %s", unwanted, got)
		}
	}
}
