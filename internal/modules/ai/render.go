package ai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// newBoundary returns a fresh, unguessable per-request delimiter token so
// untrusted content can never forge a closing marker and escape into the
// instruction context (documentation/10-ai-integration.md §5, layer 2). It
// must be generated fresh for every call — a fixed or predictable boundary
// would let a crafted repository file simply include the real one.
func newBoundary() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate boundary: %w", err)
	}
	return "GP-" + hex.EncodeToString(buf), nil
}

// renderPrompt builds the final system/user text for one call: the Vars map
// (trusted — caller-supplied metadata like a rule ID or severity, never raw
// repository content) is interpolated into the prompt's Template via
// text/template; each UntrustedBlock is wrapped in the per-request boundary
// and never touches the template engine at all (documentation/10-ai-integration.md
// §5, layers 1-3).
func renderPrompt(p Prompt, vars map[string]string, untrusted []UntrustedBlock, boundary string) (system, user string, err error) {
	tmpl, err := template.New(string(p.ID)).Parse(p.Template)
	if err != nil {
		return "", "", fmt.Errorf("parse template for %s: %w", p.ID, err)
	}

	var task strings.Builder
	if err := tmpl.Execute(&task, templateVars(vars)); err != nil {
		return "", "", fmt.Errorf("render template for %s: %w", p.ID, err)
	}

	var b strings.Builder
	b.WriteString("Task: ")
	b.WriteString(task.String())

	for _, blk := range untrusted {
		fmt.Fprintf(&b, "\n\n---BEGIN UNTRUSTED CONTENT %s (%s)---\n%s\n---END UNTRUSTED CONTENT %s---", boundary, blk.Label, blk.Content, boundary)
	}

	if len(untrusted) > 0 {
		b.WriteString("\n\nPerform the task using the content above.")
	}

	return p.System, b.String(), nil
}

// templateVars fills in an empty string for any template field the caller
// didn't set, so a missing optional Var renders as blank rather than
// text/template's default "<no value>" leaking into a prompt sent to a
// third party.
func templateVars(vars map[string]string) map[string]string {
	if vars == nil {
		return map[string]string{}
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic iteration isn't required for correctness here, but keeps golden-file tests stable
	out := make(map[string]string, len(vars))
	for _, k := range keys {
		out[k] = vars[k]
	}
	return out
}
