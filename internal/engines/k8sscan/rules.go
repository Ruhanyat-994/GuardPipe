package k8sscan

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// Rules is the Core rule catalogue documentation/05-module-specifications.md
// §9 tables for k8sscan — registered into advisory.RuleRegistry at wiring
// time (cmd/guardpipe/main.go), the same mechanism depscan.Rules already
// uses. Applied identically to raw manifests and Helm-rendered ones — no
// separate Helm rule set.
//
// Every rule's References field names the specific published standard it
// implements (documentation/05-module-specifications.md §9's grounding
// note) — Pod Security Standards is the Kubernetes project's own
// (https://kubernetes.io/docs/concepts/security/pod-security-standards/),
// CIS references are to the CIS Kubernetes Benchmark's workload-level
// controls (node/control-plane checks need live cluster access and are out
// of scope here), and NSA/CISA references are to the joint NSA/CISA
// Kubernetes Hardening Guidance.
var Rules = []domain.RuleMeta{
	// --- RBAC (highest value — this is where real cluster compromise lives) ---
	{
		ID: "k8sscan.rbac.wildcard-verbs", Title: "RBAC rule grants all verbs on a resource",
		Description: "A Role or ClusterRole rule uses verbs: [\"*\"], granting every possible action rather than the specific ones needed.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "List only the verbs the workload actually needs (get, list, watch, etc.) instead of a wildcard.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.3", "NSA/CISA Kubernetes Hardening Guidance: RBAC least privilege"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.wildcard-resources", Title: "RBAC rule grants access to all resource types",
		Description: "A Role or ClusterRole rule uses resources: [\"*\"], granting access to every resource type in the matched API groups.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "List only the specific resource types the workload needs.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.3", "NSA/CISA Kubernetes Hardening Guidance: RBAC least privilege"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.wildcard-apigroups", Title: "RBAC rule grants access across all API groups",
		Description: "A Role or ClusterRole rule uses apiGroups: [\"*\"], matching every API group rather than the specific ones needed.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "List only the specific API groups the workload needs (\"\" for core, \"apps\", etc.).",
		References:  []string{"CIS Kubernetes Benchmark 5.1.3", "NSA/CISA Kubernetes Hardening Guidance: RBAC least privilege"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.cluster-admin-binding", Title: "Subject bound to the cluster-admin ClusterRole",
		Description: "A ClusterRoleBinding or RoleBinding grants cluster-admin, the built-in role with unrestricted access to every resource in every namespace.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		Remediation: "Bind to a narrower, purpose-built Role/ClusterRole instead of cluster-admin.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.1/5.1.2", "NSA/CISA Kubernetes Hardening Guidance: RBAC least privilege"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.secrets-read-cluster-wide", Title: "ClusterRole grants cluster-wide read access to Secrets",
		Description: "A ClusterRole permits get/list/watch on secrets without scoping to a specific namespace, exposing every Secret in the cluster to whatever it's bound to.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		Remediation: "Scope Secret access with a namespaced Role instead of a ClusterRole, or restrict resourceNames to the specific Secrets actually needed.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.2", "NSA/CISA Kubernetes Hardening Guidance: secrets management"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.pod-create", Title: "RBAC rule grants permission to create Pods",
		Description: "A Role or ClusterRole grants create on pods — a known privilege-escalation path, since a created Pod can mount the cluster's own service account tokens or run privileged.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceMedium,
		Remediation: "Restrict pod-creation permission to the specific controllers/operators that legitimately need it.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: privilege escalation paths"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.escalate-bind", Title: "RBAC rule grants escalate or bind verbs on roles",
		Description: "A Role or ClusterRole grants the escalate or bind verb on roles/clusterroles, letting the holder grant itself broader permissions than it was directly assigned.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		Remediation: "Remove escalate/bind unless this subject is specifically trusted to manage RBAC itself.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.3", "NSA/CISA Kubernetes Hardening Guidance: RBAC least privilege"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.impersonate", Title: "RBAC rule grants the impersonate verb",
		Description: "A Role or ClusterRole grants impersonate, letting the holder act as any other user, group, or service account it's permitted to impersonate.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Remove impersonate unless this subject specifically needs to act on behalf of other identities.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: RBAC least privilege"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.exec-attach", Title: "RBAC rule grants pod exec or attach",
		Description: "A Role or ClusterRole grants create on pods/exec or pods/attach, letting the holder run arbitrary commands inside any matched Pod's containers.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Restrict exec/attach permission to break-glass operational roles, not general workload service accounts.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.x", "NSA/CISA Kubernetes Hardening Guidance"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.rbac.default-sa-bound", Title: "RoleBinding grants permissions to the default ServiceAccount",
		Description: "A RoleBinding or ClusterRoleBinding names the default ServiceAccount as its subject — every Pod in the namespace that doesn't explicitly request another service account uses default, so this grants the permission far more broadly than intended.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Create a dedicated ServiceAccount for the workload that actually needs this permission and bind to that instead.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.5", "NSA/CISA Kubernetes Hardening Guidance"},
		Tier:        domain.TierCore,
	},

	// --- Workload security ---
	{
		ID: "k8sscan.workload.privileged", Title: "Container runs as privileged",
		Description: "A container's securityContext sets privileged: true, giving it essentially all the capabilities of the host — equivalent to root on the node.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		Remediation: "Remove privileged: true. If specific host access is genuinely required, grant only the specific capability needed instead.",
		References:  []string{"Pod Security Standards: Baseline"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.allow-priv-escalation", Title: "Container allows privilege escalation",
		Description: "A container's securityContext does not set allowPrivilegeEscalation: false, so a process inside it can gain more privileges than its parent (e.g. via a setuid binary).",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Set securityContext.allowPrivilegeEscalation: false.",
		References:  []string{"Pod Security Standards: Restricted"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.runs-as-root", Title: "Container may run as root",
		Description: "Neither the container's nor the pod's securityContext sets runAsNonRoot: true, and/or runAsUser is 0 — the container may run as UID 0 inside its own namespace.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Set runAsNonRoot: true (and a specific non-zero runAsUser) at the pod or container securityContext.",
		References:  []string{"Pod Security Standards: Restricted"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.dangerous-capabilities", Title: "Container adds a dangerous Linux capability",
		Description: "A container's securityContext.capabilities.add includes a capability with a well-known escalation/host-impact risk (SYS_ADMIN, NET_ADMIN, SYS_PTRACE, SYS_MODULE).",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Remove the added capability unless the workload specifically requires it, and prefer the narrowest capability that satisfies the actual need.",
		References:  []string{"Pod Security Standards: Baseline (restricted capability list)"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.caps-not-dropped", Title: "Container does not drop all default capabilities",
		Description: "A container's securityContext.capabilities.drop does not include ALL — it keeps the container's full default Linux capability set instead of adding back only what's needed.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Set securityContext.capabilities.drop: [\"ALL\"], then add back only the specific capabilities actually required.",
		References:  []string{"Pod Security Standards: Restricted"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.writable-root-fs", Title: "Container's root filesystem is writable",
		Description: "A container's securityContext does not set readOnlyRootFilesystem: true — malware or a compromised process can persist or modify files anywhere in the image.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Set securityContext.readOnlyRootFilesystem: true, mounting an emptyDir volume for any directory the process genuinely needs to write to.",
		References:  []string{"Pod Security Standards: Restricted"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.host-network", Title: "Pod shares the host's network namespace",
		Description: "The pod spec sets hostNetwork: true, giving every container direct access to the node's network interfaces and any service bound to localhost on the node.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Remove hostNetwork: true unless the workload specifically requires direct node networking (e.g. a CNI plugin).",
		References:  []string{"Pod Security Standards: Baseline"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.host-pid-ipc", Title: "Pod shares the host's process or IPC namespace",
		Description: "The pod spec sets hostPID: true and/or hostIPC: true, letting containers see and interact with processes/IPC on the node itself, outside the pod's isolation.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Remove hostPID/hostIPC unless the workload specifically requires host-level process visibility (e.g. a node-monitoring agent).",
		References:  []string{"Pod Security Standards: Baseline"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.hostpath-mount", Title: "Pod mounts a hostPath volume",
		Description: "A volume of type hostPath mounts a path from the node's own filesystem into the pod — severity is elevated further for sensitive paths (/, /etc, /var/run/docker.sock).",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Use a PersistentVolumeClaim or another cluster-managed storage type instead of mounting the node's filesystem directly.",
		References:  []string{"Pod Security Standards: Baseline"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.no-resource-limits", Title: "Container has no CPU/memory limits set",
		Description: "A container declares no resources.limits — with no ceiling, a single misbehaving container can exhaust node resources and affect every other workload on it.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Set resources.limits (and resources.requests) for both CPU and memory.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: resource management"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.automount-sa-token", Title: "ServiceAccount token is automatically mounted",
		Description: "Neither the pod nor its ServiceAccount sets automountServiceAccountToken: false — every container gets a token that can authenticate to the Kubernetes API, even workloads that never call it.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Set automountServiceAccountToken: false on the pod or ServiceAccount unless the workload genuinely calls the Kubernetes API.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.5", "NSA/CISA Kubernetes Hardening Guidance"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.default-sa-used", Title: "Workload uses the default ServiceAccount",
		Description: "The pod spec doesn't set serviceAccountName, so it runs as default — sharing an identity (and any permissions bound to it) with every other unconfigured workload in the namespace.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Create and use a dedicated ServiceAccount scoped to this workload's actual needs.",
		References:  []string{"CIS Kubernetes Benchmark 5.1.5", "NSA/CISA Kubernetes Hardening Guidance"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.mutable-image-tag", Title: "Container image is referenced by a mutable tag",
		Description: "A container's image is referenced by tag (or has no tag, defaulting to \"latest\") rather than an immutable content digest (@sha256:...) — the same tag can point at different content over time.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Pin the image by digest (image@sha256:...) so a deployment always runs exactly the content that was tested.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: image security"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.no-probes", Title: "Container has no liveness or readiness probes",
		Description: "A container defines neither livenessProbe nor readinessProbe — Kubernetes has no way to detect a hung or not-yet-ready process on its own.",
		Severity:    domain.SeverityInformational, Confidence: domain.ConfidenceHigh,
		Remediation: "Add liveness and readiness probes appropriate to the workload.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: availability"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.workload.psa-level", Title: "Pod Security Standard level",
		Description: "The highest Pod Security Standard profile (privileged, baseline, or restricted) this workload's pod spec actually satisfies, with the specific controls that block a higher level.",
		Severity:    domain.SeverityInformational, Confidence: domain.ConfidenceHigh,
		Remediation: "Address the listed blocking controls to reach the next Pod Security Standard level.",
		References:  []string{"Pod Security Standards"},
		Tier:        domain.TierCore,
	},

	// --- Network and secrets ---
	{
		ID: "k8sscan.network.no-networkpolicy", Title: "Namespace has no NetworkPolicy",
		Description: "This workload's namespace has no NetworkPolicy at all — by default, every pod can be reached by every other pod in the cluster, on any port.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Add a NetworkPolicy for the namespace, starting from a default-deny baseline and explicitly allowing only the traffic each workload needs.",
		References:  []string{"CIS Kubernetes Benchmark 5.3.2", "NSA/CISA Kubernetes Hardening Guidance: network segmentation"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.network.allow-all-egress", Title: "NetworkPolicy permits unrestricted egress",
		Description: "A NetworkPolicy's egress rules include one that restricts neither `to` nor `ports` — that rule alone permits traffic to any destination on any port, regardless of how many other rules sit alongside it.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "List explicit egress rules restricting outbound traffic to the specific destinations the workload needs.",
		References:  []string{"CIS Kubernetes Benchmark 5.3.2", "NSA/CISA Kubernetes Hardening Guidance: network segmentation"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.network.nodeport-service", Title: "Service exposed via NodePort",
		Description: "A Service of type NodePort opens a port directly on every node in the cluster, bypassing any ingress/load-balancer-level access control.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Use a ClusterIP Service behind an Ingress, or a LoadBalancer Service with source-range restrictions, instead of NodePort.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: reduce exposed surface"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.network.loadbalancer-no-source-ranges", Title: "LoadBalancer Service has no source IP restriction",
		Description: "A Service of type LoadBalancer sets no loadBalancerSourceRanges, so the exposed load balancer accepts traffic from any source IP.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Set loadBalancerSourceRanges to the specific CIDR ranges that should be able to reach this service.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: reduce exposed surface"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.secrets.literal-in-manifest", Title: "Secret value committed inline in a manifest",
		Description: "A Secret resource carries data or stringData directly in a manifest tracked in source control — the same committed-secret risk depscan.secrets.* already flags for application code, at the Kubernetes-object level.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-798"},
		Remediation: "Manage this Secret's real value outside of source control — a secrets operator (External Secrets, Sealed Secrets) or applied imperatively — and remove the literal value from the tracked manifest.",
		References:  []string{"CIS Kubernetes Benchmark 5.4.1", "NSA/CISA Kubernetes Hardening Guidance: secrets management"},
		Tier:        domain.TierCore,
	},
	{
		ID: "k8sscan.secrets.env-from-secret-all", Title: "Container imports an entire Secret as environment variables",
		Description: "A container's envFrom references a Secret via secretRef, importing every key in that Secret as an environment variable — broader exposure than importing the one or two keys actually needed.",
		Severity:    domain.SeverityLow, Confidence: domain.ConfidenceHigh,
		Remediation: "Use env: with valueFrom.secretKeyRef to import only the specific keys this container needs, instead of envFrom: with secretRef.",
		References:  []string{"NSA/CISA Kubernetes Hardening Guidance: secrets management"},
		Tier:        domain.TierCore,
	},
}
