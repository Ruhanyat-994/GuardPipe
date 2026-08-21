// Package api embeds the machine-readable OpenAPI document so the binary
// can serve it (documentation/07-api-specification.md §12: "lives at
// api/openapi.yaml, served at /api/v1/openapi.yaml") without any filesystem
// dependency at runtime — required by the distroless runtime image, which
// has no shell and nothing guarantees the repo checkout is present anyway.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPISpec []byte
