package containerscan

import "strings"

// finalBaseImage returns the image a Dockerfile's final stage is built FROM,
// following stage aliases (`FROM builder`) back to the real image they name.
// It returns "" when the base can't be known without building: `scratch`, a
// reference built from an ARG (`FROM golang:${GO_VERSION}`), or no FROM at
// all. Used when there's no Docker daemon to build the repository's own
// image (the Kubernetes backend): the base image is where most of an
// image's OS-package vulnerabilities come from, and Trivy can pull it
// straight from its registry.
func finalBaseImage(dockerfile string) string {
	stages := map[string]string{} // lower-cased stage alias -> resolved image
	last := ""
	for _, raw := range strings.Split(dockerfile, "\n") {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		args := fields[1:]
		for len(args) > 0 && strings.HasPrefix(args[0], "--") { // --platform=...
			args = args[1:]
		}
		if len(args) == 0 {
			continue
		}
		image := args[0]
		if resolved, ok := stages[strings.ToLower(image)]; ok {
			image = resolved
		}
		if len(args) >= 3 && strings.EqualFold(args[1], "AS") {
			stages[strings.ToLower(args[2])] = image
		}
		last = image
	}
	if last == "" || strings.EqualFold(last, "scratch") || strings.Contains(last, "$") {
		return ""
	}
	return last
}
