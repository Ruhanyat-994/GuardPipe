package k8sscan

import "testing"

// TestWorkloadRules is the true-positive/near-miss table for fourteen of
// the fifteen k8sscan.workload.* Core rules — k8sscan.workload.psa-level
// (an always-present informational summary, not a fire/no-fire rule) has
// its own test below instead.
func TestWorkloadRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "privileged container fires", ruleID: "k8sscan.workload.privileged", wantFire: true,
			yaml: deployment(`securityContext: {privileged: true}`),
		},
		{
			name: "non-privileged container does not fire", ruleID: "k8sscan.workload.privileged", wantFire: false,
			yaml: deployment(`securityContext: {privileged: false}`),
		},
		{
			name: "privilege escalation allowed by default fires", ruleID: "k8sscan.workload.allow-priv-escalation", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "privilege escalation explicitly denied does not fire", ruleID: "k8sscan.workload.allow-priv-escalation", wantFire: false,
			yaml: deployment(`securityContext: {allowPrivilegeEscalation: false}`),
		},
		{
			name: "root not denied fires", ruleID: "k8sscan.workload.runs-as-root", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "runAsNonRoot true does not fire", ruleID: "k8sscan.workload.runs-as-root", wantFire: false,
			yaml: deployment(`securityContext: {runAsNonRoot: true}`),
		},
		{
			name: "dangerous capability added fires", ruleID: "k8sscan.workload.dangerous-capabilities", wantFire: true,
			yaml: deployment(`securityContext: {capabilities: {add: ["SYS_ADMIN"]}}`),
		},
		{
			name: "no dangerous capability added does not fire", ruleID: "k8sscan.workload.dangerous-capabilities", wantFire: false,
			yaml: deployment(`securityContext: {capabilities: {add: ["NET_BIND_SERVICE"]}}`),
		},
		{
			name: "capabilities not dropped fires", ruleID: "k8sscan.workload.caps-not-dropped", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "capabilities.drop ALL does not fire", ruleID: "k8sscan.workload.caps-not-dropped", wantFire: false,
			yaml: deployment(`securityContext: {capabilities: {drop: ["ALL"]}}`),
		},
		{
			name: "writable root filesystem fires", ruleID: "k8sscan.workload.writable-root-fs", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "read-only root filesystem does not fire", ruleID: "k8sscan.workload.writable-root-fs", wantFire: false,
			yaml: deployment(`securityContext: {readOnlyRootFilesystem: true}`),
		},
		{
			name: "hostNetwork fires", ruleID: "k8sscan.workload.host-network", wantFire: true,
			yaml: deploymentPodSpec(`hostNetwork: true
      containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`),
		},
		{
			name: "no hostNetwork does not fire", ruleID: "k8sscan.workload.host-network", wantFire: false,
			yaml: deployment(``),
		},
		{
			name: "hostPID fires", ruleID: "k8sscan.workload.host-pid-ipc", wantFire: true,
			yaml: deploymentPodSpec(`hostPID: true
      containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`),
		},
		{
			name: "no hostPID/hostIPC does not fire", ruleID: "k8sscan.workload.host-pid-ipc", wantFire: false,
			yaml: deployment(``),
		},
		{
			name: "hostPath volume fires", ruleID: "k8sscan.workload.hostpath-mount", wantFire: true,
			yaml: deploymentPodSpec(`containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]
      volumes: [{name: data, hostPath: {path: /data}}]`),
		},
		{
			name: "emptyDir volume does not fire", ruleID: "k8sscan.workload.hostpath-mount", wantFire: false,
			yaml: deploymentPodSpec(`containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]
      volumes: [{name: data, emptyDir: {}}]`),
		},
		{
			name: "no resource limits fires", ruleID: "k8sscan.workload.no-resource-limits", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "resource limits set does not fire", ruleID: "k8sscan.workload.no-resource-limits", wantFire: false,
			yaml: deployment(`resources: {limits: {cpu: "500m", memory: "256Mi"}}`),
		},
		{
			name: "no explicit automount setting fires (Kubernetes defaults to mounting)", ruleID: "k8sscan.workload.automount-sa-token", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "automountServiceAccountToken false does not fire", ruleID: "k8sscan.workload.automount-sa-token", wantFire: false,
			yaml: deploymentPodSpec(`automountServiceAccountToken: false
      containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`),
		},
		{
			name: "no serviceAccountName fires (defaults to default)", ruleID: "k8sscan.workload.default-sa-used", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "dedicated serviceAccountName does not fire", ruleID: "k8sscan.workload.default-sa-used", wantFire: false,
			yaml: deploymentPodSpec(`serviceAccountName: web-dedicated-sa
      containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`),
		},
		{
			name: "image by mutable tag fires", ruleID: "k8sscan.workload.mutable-image-tag", wantFire: true,
			yaml: deploymentPodSpec(`containers: [{name: app, image: "nginx:1.27"}]`),
		},
		{
			name: "image by digest does not fire", ruleID: "k8sscan.workload.mutable-image-tag", wantFire: false,
			yaml: deployment(``),
		},
		{
			name: "no probes configured fires", ruleID: "k8sscan.workload.no-probes", wantFire: true,
			yaml: deployment(``),
		},
		{
			name: "liveness and readiness probes configured does not fire", ruleID: "k8sscan.workload.no-probes", wantFire: false,
			yaml: deployment(`livenessProbe: {httpGet: {path: /healthz, port: 8080}}
        readinessProbe: {httpGet: {path: /ready, port: 8080}}`),
		},
	})
}

// TestPSALevel_ReflectsTheActualControlsSatisfied checks the always-present
// k8sscan.workload.psa-level summary computes the right level for three
// representative pods — a privileged one, one that clears Baseline but not
// Restricted, and one that clears Restricted.
func TestPSALevel_ReflectsTheActualControlsSatisfied(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantLevel string
	}{
		{
			name:      "privileged container is level privileged",
			yaml:      deployment(`securityContext: {privileged: true}`),
			wantLevel: "privileged",
		},
		{
			name:      "non-privileged but not hardened is level baseline",
			yaml:      deploymentPodSpec(`containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`),
			wantLevel: "baseline",
		},
		{
			name: "fully hardened is level restricted",
			yaml: deploymentPodSpec(`securityContext: {runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}
      containers:
      - name: app
        image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678
        securityContext:
          allowPrivilegeEscalation: false
          capabilities: {drop: ["ALL"]}`),
			wantLevel: "restricted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := graphFromYAML(t, tt.yaml)
			if len(graph.Manifests) != 1 {
				t.Fatalf("expected exactly one manifest, got %d", len(graph.Manifests))
			}
			level, _ := evaluatePSS(graph.Manifests[0])
			if level != tt.wantLevel {
				t.Errorf("evaluatePSS() level = %q, want %q", level, tt.wantLevel)
			}
		})
	}
}

// deployment wraps extraContainerFields — the *complete* set of extra keys
// this specific test case needs (securityContext/resources/probes/...) — as
// a single container's remaining fields, alongside a fixed name and a
// digest-pinned image. Deliberately not a "safe baseline plus one
// override": a near-miss case states the one safe field it needs, a
// true-positive case states the one unsafe field (or nothing, when the
// rule's own true-positive condition is "field absent") — never both a
// default value AND an override for the same YAML key, which would create
// a duplicate mapping key silently resolved by whichever the decoder
// prefers rather than actually testing the intended value.
func deployment(extraContainerFields string) string {
	return deploymentPodSpec(`containers:
      - name: app
        image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678
        ` + extraContainerFields)
}

// deploymentPodSpec wraps a full pod-spec-level YAML body (podSpecBody) into
// a Deployment manifest, for the pod-level rules (hostNetwork, hostPID,
// volumes, serviceAccountName, automountServiceAccountToken) that need
// control over the pod spec itself rather than just one container's fields.
func deploymentPodSpec(podSpecBody string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: default}
spec:
  template:
    spec:
      ` + podSpecBody
}
