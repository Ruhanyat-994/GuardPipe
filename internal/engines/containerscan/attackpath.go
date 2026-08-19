package containerscan

// attackContext is containerscan's counterpart to k8sscan's own
// attackContext (same reasoning: a short, deterministic "why this matters"
// sentence plus, only where a concrete escalation chain is well-established
// security knowledge, an ordered stage chain — not attempted for every one
// of Trivy's ~90 Dockerfile check IDs, only the handful with a clear,
// well-known story). A check ID with no entry here simply gets no
// impact/attack-path shown, same as any other rule this pass didn't curate.
func attackContext(trivyCheckID string) (impact string, attackPath []string) {
	switch trivyCheckID {
	case "DS002": // "Last USER should not be root"
		return "Anything that compromises this container (a code vulnerability, a supply-chain attack on a dependency) runs as root inside it by default, with every privilege the container's own capabilities allow.",
			[]string{"Application compromise", "Container runs as root", "Full in-container privilege", "Escalation if the container also has excess capabilities/mounts"}

	case "DS031": // secrets passed via build-args/envs/copied secret files
		return "A secret baked into an image layer this way is recoverable by anyone who can pull or inspect the image — including `docker history`, well after the layer that set it looks removed.", nil

	case "DS026": // no HEALTHCHECK
		return "Kubernetes/the orchestrator has no signal to detect a hung or unresponsive container on its own — a compromised container that stops responding to its intended function can still keep serving other requests.", nil

	default:
		return "", nil
	}
}
