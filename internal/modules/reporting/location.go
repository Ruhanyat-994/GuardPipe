package reporting

import (
	"fmt"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// FormatLocation renders a domain.Location's discriminated union as one
// human-readable string — the same five shapes documentation/03-architecture-overview.md
// §7.1 documents, each formatted the way a reader would actually want to see
// it (a file:line, not a JSON blob) in a report or a CSV cell.
func FormatLocation(loc domain.Location) string {
	switch loc.Type {
	case domain.LocationTypeFile:
		if loc.LineStart == 0 {
			return loc.Path
		}
		if loc.LineEnd != 0 && loc.LineEnd != loc.LineStart {
			return fmt.Sprintf("%s:%d-%d", loc.Path, loc.LineStart, loc.LineEnd)
		}
		return fmt.Sprintf("%s:%d", loc.Path, loc.LineStart)

	case domain.LocationTypeImage:
		if loc.LayerDigest != "" {
			return fmt.Sprintf("%s (layer %s)", loc.Image, shortDigest(loc.LayerDigest))
		}
		return loc.Image

	case domain.LocationTypeK8s:
		parts := []string{loc.Kind, loc.Name}
		ref := strings.Join(nonEmpty(parts), "/")
		if loc.Namespace != "" {
			ref = loc.Namespace + "/" + ref
		}
		if loc.FromHelm {
			return fmt.Sprintf("%s (%s, rendered from %s)", ref, loc.FieldPath, loc.TemplateFile)
		}
		if loc.FieldPath != "" {
			return fmt.Sprintf("%s (%s) in %s", ref, loc.FieldPath, loc.File)
		}
		return fmt.Sprintf("%s in %s", ref, loc.File)

	case domain.LocationTypeNetwork:
		host := loc.Host
		if loc.IP != "" && loc.IP != loc.Host {
			host = fmt.Sprintf("%s (%s)", loc.Host, loc.IP)
		}
		if loc.URL != "" {
			return loc.URL
		}
		if loc.Port != 0 {
			return fmt.Sprintf("%s:%d/%s", host, loc.Port, protocolLower(loc.Protocol))
		}
		return host

	case domain.LocationTypeDependency:
		ref := fmt.Sprintf("%s/%s@%s", loc.Ecosystem, loc.Package, loc.Version)
		if loc.ManifestPath != "" {
			return fmt.Sprintf("%s (%s)", ref, loc.ManifestPath)
		}
		return ref

	default:
		return "unknown location"
	}
}

func nonEmpty(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func shortDigest(digest string) string {
	const prefixLen = 19 // "sha256:" + 12 hex chars — enough to distinguish layers in a report without a full 64-char digest
	if len(digest) > prefixLen {
		return digest[:prefixLen]
	}
	return digest
}

func protocolLower(p string) string {
	if p == "" {
		return "tcp"
	}
	return strings.ToLower(p)
}
