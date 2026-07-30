package domain

// EvidenceKind matches the finding_evidence.kind CHECK constraint in
// documentation/06-database-design.md §4.12.
type EvidenceKind string

const (
	EvidenceKindCodeSnippet     EvidenceKind = "code_snippet"
	EvidenceKindCommandOutput   EvidenceKind = "command_output"
	EvidenceKindHTTPResponse    EvidenceKind = "http_response"
	EvidenceKindManifestExcerpt EvidenceKind = "manifest_excerpt"
	EvidenceKindLayerReference  EvidenceKind = "layer_reference"
)

// Evidence is a single piece of supporting material for a Finding — a code
// snippet, a command transcript, a matched value. Redacted is true once the
// engine has stripped anything secret-shaped from Value; a finding that
// detects a hardcoded credential must store the location and shape of the
// secret, never the secret itself (documentation/06-database-design.md §4.12).
type Evidence struct {
	Kind      EvidenceKind
	Value     string
	Redacted  bool
	LineStart int
	LineEnd   int
}

func (k EvidenceKind) Valid() bool {
	switch k {
	case EvidenceKindCodeSnippet, EvidenceKindCommandOutput, EvidenceKindHTTPResponse,
		EvidenceKindManifestExcerpt, EvidenceKindLayerReference:
		return true
	default:
		return false
	}
}
