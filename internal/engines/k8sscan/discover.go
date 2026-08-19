package k8sscan

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// skipDirs mirrors depscan's own list (internal/engines/depscan/secrets.go)
// — noise, not source: dependency trees, VCS internals, build output. Kept
// as a separate copy rather than exported from depscan, since the two
// engines have no dependency on each other (documentation/03-architecture-overview.md
// §6.2's dependency rule) and this list is small enough that duplicating it
// is cheaper than inventing a shared package neither engine otherwise needs.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, ".venv": true, "__pycache__": true,
}

// recognisedKinds is every Kind this engine's rules actually read — a
// manifest for any other Kind (ConfigMap, PersistentVolumeClaim, a CRD,
// Ingress, ...) is still counted as "a Kubernetes resource was found" for
// Applicable's purposes, but produces no typed sub-struct and no rule ever
// fires on it (documentation/05-module-specifications.md §9's failure-mode
// table: "CRDs / unknown kinds: generic rules only... no false claims about
// unknown schemas" — k8sscan currently has no generic image-tag/resource-limit
// rule that runs independent of a recognised workload Kind, so "generic
// rules only" today means exactly zero rules on an unrecognised Kind, not a
// gap in this pass).
var podTemplateKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true, "Job": true,
}

// discoveryResult is one file's worth of parsed manifests plus any
// documents that were recognised-but-skipped, for Run's Stats/SkipReason
// reporting.
type discoveryResult struct {
	Manifests []Manifest
	// templatedFiles is every raw YAML file (outside a Helm chart root)
	// that looks like un-rendered Helm/Go-template markup — skipped, not a
	// parse error, per §9's failure-mode table.
	TemplatedFiles []string
	// ParseErrors is file -> error, for genuinely malformed YAML.
	ParseErrors map[string]error
}

// discoverManifests walks workspaceDir for raw (non-Helm) manifests,
// skipping anything under a recognised Helm chart root — those are handled
// by renderHelmCharts instead, never double-scanned as raw templates.
func discoverManifests(workspaceDir string, chartRoots []string) (discoveryResult, error) {
	result := discoveryResult{ParseErrors: map[string]error{}}

	err := filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != workspaceDir && (skipDirs[d.Name()] || underAnyRoot(path, chartRoots)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isYAMLFile(d.Name()) {
			return nil
		}

		rel := relPath(workspaceDir, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // unreadable file — skip silently, not this engine's failure mode to report
		}

		manifests, templated, parseErr := parseDocuments(content, rel)
		if parseErr != nil {
			result.ParseErrors[rel] = parseErr
			return nil
		}
		if templated {
			result.TemplatedFiles = append(result.TemplatedFiles, rel)
			return nil
		}
		result.Manifests = append(result.Manifests, manifests...)
		return nil
	})
	if err != nil {
		return discoveryResult{}, err
	}
	return result, nil
}

// parseDocuments splits content on `---` and decodes every document that
// carries both apiVersion and kind. templated=true means the *whole file*
// looks like un-rendered template markup (found on the very first decode
// failure that contains "{{") rather than a genuine parse error — Helm
// templates are all-or-nothing per file in practice, a file is either valid
// standalone YAML or it isn't.
func parseDocuments(content []byte, file string) (manifests []Manifest, templated bool, err error) {
	dec := yaml.NewDecoder(strings.NewReader(string(content)))
	docIndex := 0
	for {
		var node yaml.Node
		decErr := dec.Decode(&node)
		if decErr != nil {
			if decErr.Error() == "EOF" {
				break
			}
			if strings.Contains(string(content), "{{") {
				return nil, true, nil
			}
			return nil, false, decErr
		}
		if len(node.Content) == 0 {
			docIndex++
			continue // empty document, e.g. a trailing "---"
		}

		m, ok := decodeManifest(&node, file, docIndex)
		if ok {
			manifests = append(manifests, m)
		}
		docIndex++
	}
	return manifests, false, nil
}

// decodeManifest decodes one YAML document node into a Manifest, or reports
// ok=false if it isn't a Kubernetes resource at all (missing apiVersion or
// kind — this is what naturally excludes CI workflow YAML, a chart's own
// values.yaml, docker-compose.yml, etc., none of which carry both fields;
// documentation/05-module-specifications.md §9's discovery step 2).
func decodeManifest(node *yaml.Node, file string, docIndex int) (Manifest, bool) {
	var id identity
	if err := node.Decode(&id); err != nil {
		return Manifest{}, false
	}
	if id.APIVersion == "" || id.Kind == "" {
		return Manifest{}, false
	}

	m := Manifest{
		APIVersion: id.APIVersion, Kind: id.Kind,
		Name: id.Metadata.Name, Namespace: id.Metadata.Namespace,
		File: file, DocIndex: docIndex, DocLine: node.Line,
	}

	switch {
	case id.Kind == "Pod":
		var w struct {
			Spec podSpec `yaml:"spec"`
		}
		if node.Decode(&w) == nil {
			m.Pod = &w.Spec
		}
	case podTemplateKinds[id.Kind]:
		var w struct {
			Spec struct {
				Template struct {
					Spec podSpec `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		if node.Decode(&w) == nil {
			m.Pod = &w.Spec.Template.Spec
		}
	case id.Kind == "CronJob":
		var w struct {
			Spec struct {
				JobTemplate struct {
					Spec struct {
						Template struct {
							Spec podSpec `yaml:"spec"`
						} `yaml:"template"`
					} `yaml:"spec"`
				} `yaml:"jobTemplate"`
			} `yaml:"spec"`
		}
		if node.Decode(&w) == nil {
			m.Pod = &w.Spec.JobTemplate.Spec.Template.Spec
		}
	case id.Kind == "Role" || id.Kind == "ClusterRole":
		var w roleSpec
		if node.Decode(&w) == nil {
			m.Role = &w
		}
	case id.Kind == "RoleBinding" || id.Kind == "ClusterRoleBinding":
		var w roleBindingSpec
		if node.Decode(&w) == nil {
			m.RoleBinding = &w
		}
	case id.Kind == "NetworkPolicy":
		var w networkPolicySpec
		if node.Decode(&w) == nil {
			m.NetworkPolicy = &w
		}
	case id.Kind == "Service":
		var w serviceSpec
		if node.Decode(&w) == nil {
			m.Service = &w
		}
	case id.Kind == "Secret":
		var w secretSpec
		if node.Decode(&w) == nil {
			m.Secret = &w
		}
	case id.Kind == "ServiceAccount":
		var w serviceAccountSpec
		if node.Decode(&w) == nil {
			m.ServiceAccount = &w
		}
	}

	return m, true
}

func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if path == root {
			return true
		}
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

func relPath(workspaceDir, path string) string {
	rel, err := filepath.Rel(workspaceDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
