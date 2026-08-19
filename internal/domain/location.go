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
	// Path is also reused by the image shape below. LineStart/LineEnd are
	// also reused by the k8s shape below (against File, not Path there) —
	// present whenever a finding can point at a specific navigable line,
	// absent when it can't (a dependency's CVE, an image-layer vulnerability
	// — nothing to redirect to in the repository, only architectural
	// context to display; see the dependency/image shapes' own fields).
	Path      string `json:"path,omitempty"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
	Column    int    `json:"column,omitempty"`

	// image — containerscan.
	Image       string `json:"image,omitempty"`
	LayerDigest string `json:"layer_digest,omitempty"`
	LayerIndex  int    `json:"layer_index,omitempty"`

	// k8s — k8sscan. FromHelm/ChartName/TemplateFile are set only when the
	// manifest came from rendering a Helm chart rather than a raw YAML
	// file — File still names a real, navigable path (the template's own
	// path under the chart root), so these three are extra context, not a
	// second location. LineStart (above) is the document's start line
	// within File for a raw manifest; deliberately 0/absent for a
	// Helm-sourced finding, since a rendered document's line numbers don't
	// correspond to TemplateFile's — better no anchor than a wrong one.
	File         string `json:"file,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Name         string `json:"name,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	Container    string `json:"container,omitempty"` // set only for a per-container finding
	FieldPath    string `json:"field_path,omitempty"`
	Value        string `json:"value,omitempty"` // the literal offending value at FieldPath, e.g. "/var/run/docker.sock"
	FromHelm     bool   `json:"from_helm,omitempty"`
	ChartName    string `json:"chart_name,omitempty"`
	TemplateFile string `json:"template_file,omitempty"`

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
