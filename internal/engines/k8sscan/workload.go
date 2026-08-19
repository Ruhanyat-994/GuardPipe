package k8sscan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// dangerousCapabilities is k8sscan.workload.dangerous-capabilities' exact
// list (documentation/05-module-specifications.md §9) — the capabilities
// with the clearest, most direct path to host compromise, not every
// capability the Pod Security Standards Baseline profile forbids (that
// fuller list is used by the PSS evaluation below instead).
var dangerousCapabilities = []string{"SYS_ADMIN", "NET_ADMIN", "SYS_PTRACE", "SYS_MODULE"}

// sensitiveHostPaths are hostPath mounts that elevate
// k8sscan.workload.hostpath-mount from high to critical — direct node root,
// the container runtime socket (full container-escape/host-compromise
// primitives), and /etc.
var sensitiveHostPaths = map[string]bool{"/": true, "/etc": true, "/var/run/docker.sock": true}

// evaluateWorkload implements the fifteen k8sscan.workload.* Core rules —
// most are per-container, three (host-network, host-pid-ipc,
// automount-sa-token/default-sa-used, psa-level) are per-pod since they're
// properties of the pod spec, not any one container in it.
func evaluateWorkload(graph *ResourceGraph, scanID uuid.UUID) []domain.Finding {
	var findings []domain.Finding

	for _, m := range graph.Manifests {
		if m.Pod == nil {
			continue
		}
		findings = append(findings, podLevelFindings(graph, scanID, m)...)

		allContainers := append(append([]container{}, m.Pod.Containers...), m.Pod.InitContainers...)
		for _, c := range allContainers {
			findings = append(findings, containerLevelFindings(scanID, m, c)...)
		}

		findings = append(findings, psaLevelFinding(scanID, m))
	}

	return findings
}

func podLevelFindings(graph *ResourceGraph, scanID uuid.UUID, m Manifest) []domain.Finding {
	var findings []domain.Finding
	pod := m.Pod

	if pod.HostNetwork {
		findings = append(findings, workloadFinding(scanID, m, "spec.template.spec.hostNetwork",
			"k8sscan.workload.host-network", "", "hostNetwork: true"))
	}
	if pod.HostPID || pod.HostIPC {
		var which []string
		if pod.HostPID {
			which = append(which, "hostPID")
		}
		if pod.HostIPC {
			which = append(which, "hostIPC")
		}
		findings = append(findings, workloadFinding(scanID, m, "spec.template.spec",
			"k8sscan.workload.host-pid-ipc", "", strings.Join(which, ",")))
	}
	for i, v := range pod.Volumes {
		if v.HostPath == nil {
			continue
		}
		f := workloadFinding(scanID, m, fmt.Sprintf("spec.template.spec.volumes[%d].hostPath.path", i),
			"k8sscan.workload.hostpath-mount", "", v.HostPath.Path)
		if sensitiveHostPaths[v.HostPath.Path] {
			f.Severity = domain.SeverityCritical
			f.Description = fmt.Sprintf("Mounts %s, a sensitive host path — this is more than filesystem access, it's a direct path to host/container-runtime compromise.", v.HostPath.Path)
		}
		findings = append(findings, f)
	}

	// effectiveAutomountToken true means "not explicitly disabled anywhere"
	// — Kubernetes defaults to mounting the token.
	if effectiveAutomountToken(graph, m) {
		findings = append(findings, workloadFinding(scanID, m, "spec.template.spec.automountServiceAccountToken",
			"k8sscan.workload.automount-sa-token", "", "not disabled"))
	}

	if pod.ServiceAccountName == "" || pod.ServiceAccountName == "default" {
		findings = append(findings, workloadFinding(scanID, m, "spec.template.spec.serviceAccountName",
			"k8sscan.workload.default-sa-used", "", "default"))
	}

	return findings
}

// effectiveAutomountToken reports whether the token *would* be mounted:
// true unless the pod itself, or (when the pod doesn't say) its
// ServiceAccount, explicitly sets automountServiceAccountToken: false.
func effectiveAutomountToken(graph *ResourceGraph, m Manifest) bool {
	if m.Pod.AutomountServiceAccountToken != nil {
		return *m.Pod.AutomountServiceAccountToken
	}
	saName := m.Pod.ServiceAccountName
	if saName == "" {
		saName = "default"
	}
	for _, other := range graph.Manifests {
		if other.ServiceAccount == nil || other.Namespace != m.Namespace || other.Name != saName {
			continue
		}
		if other.ServiceAccount.AutomountServiceAccountToken != nil {
			return *other.ServiceAccount.AutomountServiceAccountToken
		}
	}
	return true // Kubernetes' own default
}

func containerLevelFindings(scanID uuid.UUID, m Manifest, c container) []domain.Finding {
	var findings []domain.Finding
	sc := c.SecurityContext

	if sc != nil && sc.Privileged != nil && *sc.Privileged {
		findings = append(findings, containerFinding(scanID, m, c, "securityContext.privileged",
			"k8sscan.workload.privileged", "true"))
	}
	if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		findings = append(findings, containerFinding(scanID, m, c, "securityContext.allowPrivilegeEscalation",
			"k8sscan.workload.allow-priv-escalation", "not false"))
	}
	if runsAsRoot(m, c) {
		findings = append(findings, containerFinding(scanID, m, c, "securityContext.runAsNonRoot",
			"k8sscan.workload.runs-as-root", "not enforced"))
	}
	if sc != nil && sc.Capabilities != nil {
		if added := addedDangerousCapabilities(sc.Capabilities.Add); len(added) > 0 {
			findings = append(findings, containerFinding(scanID, m, c, "securityContext.capabilities.add",
				"k8sscan.workload.dangerous-capabilities", strings.Join(added, ",")))
		}
	}
	if sc == nil || sc.Capabilities == nil || !slices.Contains(sc.Capabilities.Drop, "ALL") {
		findings = append(findings, containerFinding(scanID, m, c, "securityContext.capabilities.drop",
			"k8sscan.workload.caps-not-dropped", "does not include ALL"))
	}
	if sc == nil || sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		findings = append(findings, containerFinding(scanID, m, c, "securityContext.readOnlyRootFilesystem",
			"k8sscan.workload.writable-root-fs", "not true"))
	}
	if c.Resources == nil || len(c.Resources.Limits) == 0 {
		findings = append(findings, containerFinding(scanID, m, c, "resources.limits",
			"k8sscan.workload.no-resource-limits", "unset"))
	}
	if !hasImageDigest(c.Image) {
		findings = append(findings, containerFinding(scanID, m, c, "image",
			"k8sscan.workload.mutable-image-tag", c.Image))
	}
	if !c.LivenessProbe.set && !c.ReadinessProbe.set {
		findings = append(findings, containerFinding(scanID, m, c, "",
			"k8sscan.workload.no-probes", "neither configured"))
	}

	return findings
}

// runsAsRoot resolves the effective runAsNonRoot/runAsUser — a container's
// own securityContext overrides the pod's, matching Kubernetes' own
// override semantics — and reports true if neither ends up enforcing
// non-root.
func runsAsRoot(m Manifest, c container) bool {
	var nonRoot *bool
	var user *int64
	if c.SecurityContext != nil {
		nonRoot, user = c.SecurityContext.RunAsNonRoot, c.SecurityContext.RunAsUser
	}
	if nonRoot == nil && m.Pod.SecurityContext != nil {
		nonRoot = m.Pod.SecurityContext.RunAsNonRoot
	}
	if user == nil && m.Pod.SecurityContext != nil {
		user = m.Pod.SecurityContext.RunAsUser
	}
	if user != nil && *user == 0 {
		return true
	}
	return nonRoot == nil || !*nonRoot
}

func addedDangerousCapabilities(add []string) []string {
	var found []string
	for _, cap := range add {
		if slices.Contains(dangerousCapabilities, strings.ToUpper(cap)) {
			found = append(found, cap)
		}
	}
	return found
}

// hasImageDigest reports whether image is pinned by content digest
// (@sha256:...) rather than a mutable tag (or no tag at all, which
// defaults to :latest).
func hasImageDigest(image string) bool {
	return strings.Contains(image, "@sha256:")
}

func containerFinding(scanID uuid.UUID, m Manifest, c container, fieldSuffix, ruleID, evidence string) domain.Finding {
	fieldPath := "spec.template.spec.containers[" + c.Name + "]"
	if fieldSuffix != "" {
		fieldPath += "." + fieldSuffix
	}
	meta := ruleMetaByID(ruleID)
	normLoc := k8sNormalizedLocation(m, "container:"+c.Name+":"+fieldSuffix)
	f := domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineK8sScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, evidence),
		Title:       fmt.Sprintf("%s (%s/%s, container %q)", meta.Title, m.Kind, resourceDisplayName(m), c.Name),
		Description: meta.Description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    k8sLocation(m, fieldPath, c.Name, evidence),
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
	}
	return withAttackContext(f, ruleID, evidence)
}

func workloadFinding(scanID uuid.UUID, m Manifest, fieldPath, ruleID, descriptionOverride, evidence string) domain.Finding {
	meta := ruleMetaByID(ruleID)
	description := meta.Description
	if descriptionOverride != "" {
		description = descriptionOverride
	}
	normLoc := k8sNormalizedLocation(m, fieldPath)
	f := domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineK8sScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, evidence),
		Title:       fmt.Sprintf("%s (%s/%s)", meta.Title, m.Kind, resourceDisplayName(m)),
		Description: description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    k8sLocation(m, fieldPath, "", evidence),
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
	}
	return withAttackContext(f, ruleID, evidence)
}

// --- Pod Security Standards evaluation (FR-K8S-008) ---

// pssControl is one specific check the Pod Security Standards profiles
// depend on, paired with the human-readable reason shown when it's what
// blocked a workload from reaching a higher level.
type pssControl struct {
	Name   string
	Failed bool
	Reason string
}

// evaluatePSS is a deliberate, documented approximation of the official Pod
// Security Standards — it reuses exactly the signals the other fourteen
// workload rules already compute (this function doesn't introduce new
// detection logic), not a from-scratch reimplementation of every control in
// the full upstream spec (procMount, sysctls, supplementalGroups, and a few
// others aren't checked). Baseline/Restricted here means "passes the subset
// of Baseline/Restricted this engine actually evaluates," stated as such in
// the finding, not a certification against the complete standard.
func evaluatePSS(m Manifest) (level string, blocking []string) {
	allContainers := append(append([]container{}, m.Pod.Containers...), m.Pod.InitContainers...)

	baseline := []pssControl{
		{"hostNetwork", m.Pod.HostNetwork, "hostNetwork is true"},
		{"hostPID", m.Pod.HostPID, "hostPID is true"},
		{"hostIPC", m.Pod.HostIPC, "hostIPC is true"},
		{"hostPath volume", hasHostPathVolume(m.Pod), "a hostPath volume is mounted"},
		{"privileged container", anyContainerPrivileged(allContainers), "a container runs privileged"},
		{"dangerous capability", anyContainerHasDangerousCapability(allContainers), "a container adds a dangerous capability"},
	}
	var baselineBlocking []string
	for _, c := range baseline {
		if c.Failed {
			baselineBlocking = append(baselineBlocking, c.Reason)
		}
	}
	if len(baselineBlocking) > 0 {
		return "privileged", baselineBlocking
	}

	restricted := []pssControl{
		{"allowPrivilegeEscalation", !allContainersDenyPrivEsc(allContainers), "not every container sets allowPrivilegeEscalation: false"},
		{"runAsNonRoot", !allContainersRunAsNonRoot(m, allContainers), "not every container enforces runAsNonRoot"},
		{"capabilities.drop", !allContainersDropAll(allContainers), "not every container drops all default capabilities"},
		{"seccompProfile", !hasRuntimeDefaultSeccomp(m.Pod), "no RuntimeDefault/Localhost seccomp profile is set"},
	}
	var restrictedBlocking []string
	for _, c := range restricted {
		if c.Failed {
			restrictedBlocking = append(restrictedBlocking, c.Reason)
		}
	}
	if len(restrictedBlocking) > 0 {
		return "baseline", restrictedBlocking
	}
	return "restricted", nil
}

func hasHostPathVolume(pod *podSpec) bool {
	for _, v := range pod.Volumes {
		if v.HostPath != nil {
			return true
		}
	}
	return false
}

func anyContainerPrivileged(containers []container) bool {
	for _, c := range containers {
		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			return true
		}
	}
	return false
}

func anyContainerHasDangerousCapability(containers []container) bool {
	for _, c := range containers {
		if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil &&
			len(addedDangerousCapabilities(c.SecurityContext.Capabilities.Add)) > 0 {
			return true
		}
	}
	return false
}

func allContainersDenyPrivEsc(containers []container) bool {
	for _, c := range containers {
		if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
			return false
		}
	}
	return true
}

func allContainersRunAsNonRoot(m Manifest, containers []container) bool {
	for _, c := range containers {
		if runsAsRoot(m, c) {
			return false
		}
	}
	return true
}

func allContainersDropAll(containers []container) bool {
	for _, c := range containers {
		if c.SecurityContext == nil || c.SecurityContext.Capabilities == nil || !slices.Contains(c.SecurityContext.Capabilities.Drop, "ALL") {
			return false
		}
	}
	return true
}

func hasRuntimeDefaultSeccomp(pod *podSpec) bool {
	if pod.SecurityContext != nil && pod.SecurityContext.SeccompProfile != nil {
		t := pod.SecurityContext.SeccompProfile.Type
		if t == "RuntimeDefault" || t == "Localhost" {
			return true
		}
	}
	for _, c := range pod.Containers {
		if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
			t := c.SecurityContext.SeccompProfile.Type
			if t == "RuntimeDefault" || t == "Localhost" {
				return true
			}
		}
	}
	return false
}

func psaLevelFinding(scanID uuid.UUID, m Manifest) domain.Finding {
	const ruleID = "k8sscan.workload.psa-level"
	level, blocking := evaluatePSS(m)
	meta := ruleMetaByID(ruleID)

	description := fmt.Sprintf("This workload satisfies Pod Security Standard level %q.", level)
	if len(blocking) > 0 {
		description = fmt.Sprintf("This workload satisfies Pod Security Standard level %q. Blocked from the next level by: %s.",
			level, strings.Join(blocking, "; "))
	}

	normLoc := k8sNormalizedLocation(m, "psa-level")
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineK8sScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, level),
		Title:       fmt.Sprintf("Pod Security Standard level: %s (%s/%s)", level, m.Kind, resourceDisplayName(m)),
		Description: description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		Location:    k8sLocation(m, "spec.template.spec", "", ""),
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
		Metadata:    map[string]any{"psa_level": level},
	}
}
