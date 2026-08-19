package cicdscan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// discoveryResult is every workflow this scan found, plus anything that
// couldn't be read as one — documentation/05-module-specifications.md §10's
// failure-mode table: "Invalid workflow YAML: rule pass skips the file with
// parse_error; AI pass still runs on the raw text" — so ParseErrors carries
// the raw content alongside the error, not just the error.
type discoveryResult struct {
	Workflows   []Workflow
	ParseErrors map[string]error // file -> error; RawContent has the bytes for the AI pass anyway
	RawContent  map[string][]byte
}

// discoverWorkflows finds every `.github/workflows/*.yml`/`*.yaml` in the
// workspace — GitHub only ever reads workflows from that exact directory
// (unlike k8sscan's manifests, which can live anywhere), so there is no
// broader repository walk needed, only that one directory.
func discoverWorkflows(workspaceDir string) (discoveryResult, error) {
	result := discoveryResult{ParseErrors: map[string]error{}, RawContent: map[string][]byte{}}
	dir := filepath.Join(workspaceDir, ".github", "workflows")

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return discoveryResult{}, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		rel := relPath(workspaceDir, path)

		content, err := os.ReadFile(path)
		if err != nil {
			continue // unreadable file — skip silently, same as k8sscan's own discovery
		}
		result.RawContent[rel] = content

		wf, err := parseWorkflow(content, rel)
		if err != nil {
			result.ParseErrors[rel] = err
			continue
		}
		result.Workflows = append(result.Workflows, wf)
	}
	return result, nil
}

// parseWorkflow decodes one workflow file's top-level keys by walking its
// yaml.Node tree directly rather than a struct-tag Decode — deliberately,
// to sidestep YAML 1.1's implicit-boolean resolution of the bare `on`/`off`
// scalars (a well-known gotcha for this exact file format: an unquoted
// `on:` key can resolve to the boolean true rather than the string "on"
// depending on the parser's resolver), and so every node's own .Line is
// still available for Job/Step line tracking without a second parse pass.
func parseWorkflow(content []byte, file string) (Workflow, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return Workflow{}, err
	}
	if len(doc.Content) == 0 {
		return Workflow{File: file, Line: 1}, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return Workflow{File: file, Line: 1}, nil
	}

	wf := Workflow{File: file, Line: 1}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		switch key.Value {
		case "name":
			wf.Name = val.Value
		case "on":
			wf.Triggers = parseTriggers(val)
		case "permissions":
			wf.Permissions = parsePermissions(val)
			wf.PermissionsLine = key.Line
		case "jobs":
			wf.Jobs = parseJobs(val)
		}
	}
	wf.WorkflowRunDownloadsArtifact = looksLikeWorkflowRunArtifactDownload(content, wf.Triggers)
	return wf, nil
}

// parseTriggers handles all three shapes `on:` can take.
func parseTriggers(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{node.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range node.Content {
			if c.Kind == yaml.ScalarNode {
				out = append(out, c.Value)
			}
		}
		return out
	case yaml.MappingNode:
		var out []string
		for i := 0; i+1 < len(node.Content); i += 2 {
			out = append(out, node.Content[i].Value)
		}
		return out
	default:
		return nil
	}
}

// parsePermissions handles both shapes `permissions:` can take.
func parsePermissions(node *yaml.Node) Permissions {
	p := Permissions{Present: true}
	switch node.Kind {
	case yaml.ScalarNode:
		switch node.Value {
		case "write-all":
			p.WriteAll = true
		case "read-all":
			p.ReadAll = true
		}
	case yaml.MappingNode:
		p.Scopes = map[string]string{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			p.Scopes[node.Content[i].Value] = node.Content[i+1].Value
		}
	}
	return p
}

func parseJobs(node *yaml.Node) []Job {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var jobs []Job
	for i := 0; i+1 < len(node.Content); i += 2 {
		idKey, jobNode := node.Content[i], node.Content[i+1]
		if jobNode.Kind != yaml.MappingNode {
			continue
		}
		job := Job{ID: idKey.Value, Line: idKey.Line}
		for j := 0; j+1 < len(jobNode.Content); j += 2 {
			key, val := jobNode.Content[j], jobNode.Content[j+1]
			switch key.Value {
			case "runs-on":
				job.RunsOn = parseRunsOn(val)
			case "container":
				job.Container = parseContainerImage(val)
			case "permissions":
				job.Permissions = parsePermissions(val)
				job.PermissionsLine = key.Line
			case "uses":
				job.Uses = val.Value
			case "secrets":
				if val.Kind == yaml.ScalarNode && val.Value == "inherit" {
					job.SecretsInherit = true
				}
			case "steps":
				job.Steps = parseSteps(val)
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func parseRunsOn(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{node.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range node.Content {
			if c.Kind == yaml.ScalarNode {
				out = append(out, c.Value)
			}
		}
		return out
	default:
		return nil
	}
}

func parseContainerImage(node *yaml.Node) string {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "image" {
				return node.Content[i+1].Value
			}
		}
	}
	return ""
}

func parseSteps(node *yaml.Node) []Step {
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	var steps []Step
	for _, stepNode := range node.Content {
		if stepNode.Kind != yaml.MappingNode {
			continue
		}
		step := Step{Line: stepNode.Line}
		for i := 0; i+1 < len(stepNode.Content); i += 2 {
			key, val := stepNode.Content[i], stepNode.Content[i+1]
			switch key.Value {
			case "name":
				step.Name = val.Value
			case "uses":
				step.Uses = val.Value
			case "run":
				step.Run = val.Value
			case "if":
				step.If = val.Value
			case "with":
				step.With = parseScalarMap(val)
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// looksLikeWorkflowRunArtifactDownload is a raw-text heuristic for
// cicdscan.trigger.workflow-run-untrusted: a workflow_run-triggered
// workflow that also mentions downloading an artifact and references the
// triggering run's own id/context anywhere in the file. Deliberately a
// text search, not a structured check of exactly which step's `with:`
// fields do what — the point is "this workflow looks like the well-known
// workflow_run-plus-artifact-download pattern," not a precise dataflow
// proof, the same honesty tradeoff k8sscan's own approximated rules (e.g.
// its Pod Security Standards evaluation) already document.
func looksLikeWorkflowRunArtifactDownload(content []byte, triggers []string) bool {
	if !slices.Contains(triggers, "workflow_run") {
		return false
	}
	text := string(content)
	return strings.Contains(text, "download-artifact") && strings.Contains(text, "github.event.workflow_run")
}

// parseScalarMap flattens a mapping node's scalar-valued entries only —
// used for a step's `with:` block, where the rules this engine has only
// ever need a handful of simple string values (e.g. checkout's `ref:`), not
// arbitrary nested with: shapes.
func parseScalarMap(node *yaml.Node) map[string]string {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	out := map[string]string{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i], node.Content[i+1]
		if val.Kind == yaml.ScalarNode {
			out[key.Value] = val.Value
		}
	}
	return out
}

func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

func relPath(workspaceDir, path string) string {
	rel, err := filepath.Rel(workspaceDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
