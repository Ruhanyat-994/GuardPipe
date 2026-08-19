package k8sscan

// attackContext returns a short, deterministic "why this matters" sentence
// (Finding.Metadata["impact"]) and, only for the rules where a concrete
// escalation chain is well-established security knowledge rather than
// something invented per finding, an ordered chain of stage names a
// frontend can render as a simple path (Finding.Metadata["attack_path"]) —
// e.g. "Pod compromise -> Docker socket -> Docker API access -> ...".
// Deliberately the same discipline as Remediation
// (documentation/03-architecture-overview.md §7.1: deterministic, stands
// alone without AI) applied to "why," not just "how to fix": a fixed,
// reviewed narrative per rule, not a generated one. A rule with no entry
// here simply gets no impact/attack-path shown — an absent explanation is
// honest; a fabricated one is not, so this is deliberately not exhaustive
// over all 31 Core rules, only the ones with a well-known escalation story
// (hostPath/Docker socket, privileged, hostNetwork/PID/IPC, dangerous
// capabilities, cluster-admin, wildcard/escalating RBAC, secret exposure).
func attackContext(ruleID, value string) (impact string, attackPath []string) {
	switch ruleID {
	case "k8sscan.workload.hostpath-mount":
		if value == "/var/run/docker.sock" {
			return "The container can talk to the host's Docker daemon through the mounted socket — equivalent to root on the node.",
				[]string{"Pod compromise", "Docker socket", "Docker API access", "Create a privileged container", "Host filesystem access", "Node compromise"}
		}
		return "The container can read (and, unless the mount is read-only, write) the host's filesystem at this path, outside any namespace isolation.",
			[]string{"Pod compromise", "Host filesystem access", "Node compromise"}

	case "k8sscan.workload.privileged":
		return "A privileged container disables nearly every container isolation boundary — it can access host devices, modify kernel parameters, and load kernel modules.",
			[]string{"Pod compromise", "Privileged container", "Host device / kernel access", "Node compromise"}

	case "k8sscan.workload.host-network":
		return "The Pod shares the node's network namespace, so it can see and intercept traffic meant for the host and other Pods, and bind to host ports directly.",
			[]string{"Pod compromise", "Host network namespace", "Traffic interception / lateral movement"}

	case "k8sscan.workload.host-pid-ipc":
		return "The Pod shares the host's process and/or IPC namespace, so it can see and signal every process on the node, including ones outside any container.",
			[]string{"Pod compromise", "Host process namespace", "Interact with host processes", "Node compromise"}

	case "k8sscan.workload.dangerous-capabilities":
		return "This capability grants direct kernel-level access (device access, network configuration, or tracing other processes) well beyond what a typical application needs.",
			[]string{"Pod compromise", "Dangerous capability", "Kernel-level access", "Node compromise"}

	case "k8sscan.rbac.cluster-admin-binding":
		return "cluster-admin can read, create, modify, or delete every resource in every namespace, including Secrets and Nodes — there is no broader grant in Kubernetes.",
			[]string{"Credential / token compromise", "cluster-admin binding", "Full API access", "Cluster-wide compromise"}

	case "k8sscan.rbac.wildcard-verbs", "k8sscan.rbac.wildcard-resources", "k8sscan.rbac.wildcard-apigroups":
		return "A wildcard grant is equivalent to cluster-admin for anything this rule's scope covers — any resource type added later is automatically included too.",
			[]string{"Credential / token compromise", "Wildcard RBAC grant", "Broad API access"}

	case "k8sscan.rbac.secrets-read-cluster-wide":
		return "Any Secret in any namespace — database credentials, TLS keys, other ServiceAccount tokens — becomes readable, often enough on its own to pivot to full cluster-admin.",
			[]string{"Credential / token compromise", "Cluster-wide Secret read", "Credential harvesting", "Further privilege escalation"}

	case "k8sscan.rbac.escalate-bind":
		return "escalate/bind on Roles lets the holder grant itself any permission any Role could carry — a direct, self-service path to full cluster-admin.",
			[]string{"Credential / token compromise", "escalate/bind on roles", "Self-granted cluster-admin"}

	case "k8sscan.rbac.impersonate":
		return "impersonate lets the holder act as any other user, group, or ServiceAccount — including ones with far more privilege than the caller itself has.",
			[]string{"Credential / token compromise", "impersonate verb", "Act as a higher-privileged identity"}

	case "k8sscan.rbac.exec-attach":
		return "pods/exec and pods/attach let the holder run arbitrary commands inside any matching Pod — equivalent to compromising every one of those Pods directly.",
			[]string{"Credential / token compromise", "pod exec/attach", "Arbitrary code execution in-cluster"}

	case "k8sscan.secrets.literal-in-manifest", "k8sscan.secrets.env-from-secret-all":
		return "Anyone with read access to this manifest (or this Secret) — including CI systems, other teams, or a compromised low-privilege workload — gets the credential too.", nil

	default:
		return "", nil
	}
}
