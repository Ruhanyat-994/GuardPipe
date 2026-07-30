package domain

// LocationType discriminates which shape of Location is populated. Exactly
// one of the five shapes is meaningful for a given Type — the others are
// left at their zero value. This mirrors the `location` JSONB contract in
// documentation/06-database-design.md §5 field-for-field, so a Location
// marshals to exactly the JSON shape the store and the frontend expect.
type LocationType string

const (
	LocationTypeFile       LocationType = "file"
	LocationTypeImage      LocationType = "image"
	LocationTypeK8s        LocationType = "k8s"
	LocationTypeNetwork    LocationType = "network"
	LocationTypeDependency LocationType = "dependency"
)

// Location is a discriminated union addressing where a Finding was found:
// a source file line, a container image layer, a Kubernetes manifest field,
// a network service, or a dependency manifest entry.
type Location struct {
	Type LocationType `json:"type"`

	// file — codescan, depscan, cicdscan, docreview.
	// Path is also reused by the image shape below.
	Path      string `json:"path,omitempty"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
	Column    int    `json:"column,omitempty"`

	// image — containerscan.
	Image       string `json:"image,omitempty"`
	LayerDigest string `json:"layer_digest,omitempty"`
	LayerIndex  int    `json:"layer_index,omitempty"`

	// k8s — k8sscan.
	File      string `json:"file,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	FieldPath string `json:"field_path,omitempty"`

	// network — pentest.
	Host     string `json:"host,omitempty"`
	IP       string `json:"ip,omitempty"`
	Port     int    `json:"port,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Service  string `json:"service,omitempty"`
	URL      string `json:"url,omitempty"`

	// dependency — depscan.
	Ecosystem    string `json:"ecosystem,omitempty"`
	Package      string `json:"package,omitempty"`
	Version      string `json:"version,omitempty"`
	ManifestPath string `json:"manifest_path,omitempty"`
}

func (t LocationType) Valid() bool {
	switch t {
	case LocationTypeFile, LocationTypeImage, LocationTypeK8s, LocationTypeNetwork, LocationTypeDependency:
		return true
	default:
		return false
	}
}
