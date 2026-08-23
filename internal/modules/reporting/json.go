package reporting

import "encoding/json"

// RenderJSON marshals a report as the Core export format
// (documentation/07-api-specification.md §5: "full scan: metadata, all
// findings, score breakdown, engine results" — the score-breakdown part is
// omitted honestly, see types.go's package doc, since modules/scoring isn't
// built).
func RenderJSON(data *ReportData) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
