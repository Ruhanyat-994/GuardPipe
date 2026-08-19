// Package k8sscan is GuardPipe's own Kubernetes manifest policy engine
// (documentation/05-module-specifications.md §9, BUILD_GUIDE.md Phase 9) —
// per ADR-0010, it is not a wrapper around kube-bench/kube-score/Polaris or
// any other third-party tool, unlike codescan/containerscan. It never
// connects to a live cluster, needs no kubeconfig, and reads only what's
// already in the cloned workspace: raw manifests, plus any Helm chart
// rendered offline first (helm.go). Every rule's doc comment in rules.go
// cites the named standard it implements — the Kubernetes project's own Pod
// Security Standards, the CIS Kubernetes Benchmark's workload-level
// controls, or the NSA/CISA Kubernetes Hardening Guidance.
package k8sscan

// --- narrow, policy-relevant subset of the Kubernetes API shapes ---
//
// These are hand-written, yaml-tagged to match the real API's field names
// and nesting, not k8s.io/api types — this engine only ever reads a small
// slice of fields for policy checks, never round-trips or validates a full
// object, so pulling in the full upstream API/apimachinery dependency tree
// (already unavoidable transitively through the Helm SDK, see helm.go)
// just to re-parse YAML whose shape we already know would buy nothing.
// Pointers are used wherever "unset" and "false" must be distinguishable
// (e.g. runAsNonRoot unset vs. explicitly false).

// identity is every Kubernetes object's common envelope — decoded first, on
// its own, to decide whether a YAML document is a Kubernetes resource at
// all (both apiVersion and kind present) and which typed shape to decode it
// into next.
type identity struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

// Manifest is one parsed Kubernetes object, plus where it came from. Exactly
// one of Pod/Role/RoleBinding/NetworkPolicy/Service/Secret/ServiceAccount is
// non-nil, matching Kind — everything else is a recognised-but-uninteresting
// kind (a ConfigMap, a CRD, ...) that discovery still records for
// completeness but no rule family reads.
type Manifest struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string // "" means the manifest doesn't pin one (defaults to "default" at apply time)

	// Source location — always set.
	File     string // relative to the workspace root
	DocIndex int    // which `---`-separated document within File, 0-based

	// DocLine is the 1-based line File's YAML parser reported for this
	// document's start — good enough to jump straight to the right resource
	// in a multi-document file, even without field-level precision (see
	// rules.go/findings.go's k8sLocation). Only trustworthy for a raw
	// manifest, where File really is what was parsed: renderedChartManifests
	// (helm.go) clears this back to 0 for a Helm-sourced Manifest, since the
	// line numbers that produced it are the *rendered* output's, not
	// File/TemplateFile's — the same known limitation renderHelmChart's own
	// doc comment already describes for line numbers generally.
	DocLine int

	// Source template — set only when this manifest came from rendering a
	// Helm chart (helm.go), so a finding can point at the template a human
	// actually needs to edit, not the ephemeral rendered YAML.
	FromHelm     bool
	ChartName    string
	TemplateFile string

	Pod            *podSpec
	Role           *roleSpec
	RoleBinding    *roleBindingSpec
	NetworkPolicy  *networkPolicySpec
	Service        *serviceSpec
	Secret         *secretSpec
	ServiceAccount *serviceAccountSpec
}

// podSpec is the subset of a pod template's spec every workload rule reads,
// extracted from whichever nesting depth its owning Kind uses (Pod: spec
// directly; Deployment/StatefulSet/DaemonSet/ReplicaSet/Job:
// spec.template.spec; CronJob: spec.jobTemplate.spec.template.spec — see
// discover.go's decodeManifest).
type podSpec struct {
	Containers                   []container         `yaml:"containers"`
	InitContainers               []container         `yaml:"initContainers"`
	Volumes                      []volume            `yaml:"volumes"`
	HostNetwork                  bool                `yaml:"hostNetwork"`
	HostPID                      bool                `yaml:"hostPID"`
	HostIPC                      bool                `yaml:"hostIPC"`
	ServiceAccountName           string              `yaml:"serviceAccountName"`
	AutomountServiceAccountToken *bool               `yaml:"automountServiceAccountToken"`
	SecurityContext              *podSecurityContext `yaml:"securityContext"`
}

type podSecurityContext struct {
	RunAsNonRoot   *bool               `yaml:"runAsNonRoot"`
	RunAsUser      *int64              `yaml:"runAsUser"`
	SeccompProfile *seccompProfileSpec `yaml:"seccompProfile"`
}

type container struct {
	Name            string           `yaml:"name"`
	Image           string           `yaml:"image"`
	SecurityContext *securityContext `yaml:"securityContext"`
	Resources       *resourceReqs    `yaml:"resources"`
	Env             []envVar         `yaml:"env"`
	EnvFrom         []envFromSource  `yaml:"envFrom"`
	Ports           []containerPort  `yaml:"ports"`
	LivenessProbe   yamlAny          `yaml:"livenessProbe"`
	ReadinessProbe  yamlAny          `yaml:"readinessProbe"`
}

// yamlAny decodes any present-but-arbitrary-shaped field just to detect
// presence (e.g. a probe's exact exec/httpGet/tcpSocket shape is irrelevant
// to k8sscan.workload.no-probes — only "is one configured at all" is).
type yamlAny struct{ set bool }

func (y *yamlAny) UnmarshalYAML(unmarshal func(any) error) error {
	var v any
	if err := unmarshal(&v); err != nil {
		return err
	}
	y.set = v != nil
	return nil
}

type securityContext struct {
	Privileged               *bool               `yaml:"privileged"`
	AllowPrivilegeEscalation *bool               `yaml:"allowPrivilegeEscalation"`
	RunAsNonRoot             *bool               `yaml:"runAsNonRoot"`
	RunAsUser                *int64              `yaml:"runAsUser"`
	ReadOnlyRootFilesystem   *bool               `yaml:"readOnlyRootFilesystem"`
	Capabilities             *capabilitiesSpec   `yaml:"capabilities"`
	SeccompProfile           *seccompProfileSpec `yaml:"seccompProfile"`
}

type capabilitiesSpec struct {
	Add  []string `yaml:"add"`
	Drop []string `yaml:"drop"`
}

type seccompProfileSpec struct {
	Type string `yaml:"type"`
}

type resourceReqs struct {
	Limits   map[string]string `yaml:"limits"`
	Requests map[string]string `yaml:"requests"`
}

type envVar struct {
	Name      string        `yaml:"name"`
	ValueFrom *envVarSource `yaml:"valueFrom"`
}

type envVarSource struct {
	SecretKeyRef yamlAny `yaml:"secretKeyRef"`
}

type envFromSource struct {
	SecretRef yamlAny `yaml:"secretRef"`
}

type containerPort struct {
	ContainerPort int `yaml:"containerPort"`
	HostPort      int `yaml:"hostPort"`
}

type volume struct {
	Name     string        `yaml:"name"`
	HostPath *hostPathSpec `yaml:"hostPath"`
}

type hostPathSpec struct {
	Path string `yaml:"path"`
}

// roleSpec is a Role or ClusterRole — Manifest.Kind already distinguishes
// which, so this one type covers both.
type roleSpec struct {
	Rules []policyRule `yaml:"rules"`
}

type policyRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

// roleBindingSpec is a RoleBinding or ClusterRoleBinding.
type roleBindingSpec struct {
	RoleRef  roleRef   `yaml:"roleRef"`
	Subjects []subject `yaml:"subjects"`
}

type roleRef struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

type subject struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type networkPolicySpec struct {
	Spec struct {
		PodSelector yamlAny  `yaml:"podSelector"`
		PolicyTypes []string `yaml:"policyTypes"`
		// Egress being present-but-empty ([]) is Kubernetes' own
		// "deny all egress" idiom — a *good* default-deny posture, the
		// opposite of what k8sscan.network.allow-all-egress flags. What the
		// rule actually looks for is an individual rule *within* this list
		// that restricts neither `to` nor `ports` — that specific rule
		// permits egress to anywhere, on any port, regardless of how many
		// other, more restrictive rules sit alongside it.
		Egress []networkPolicyRule `yaml:"egress"`
	} `yaml:"spec"`
}

type networkPolicyRule struct {
	To    yamlAny `yaml:"to"`
	Ports yamlAny `yaml:"ports"`
}

// unrestricted reports whether this egress rule permits traffic to any
// destination on any port — neither `to` nor `ports` narrows it at all.
func (r networkPolicyRule) unrestricted() bool {
	return !r.To.set && !r.Ports.set
}

type serviceSpec struct {
	Spec struct {
		Type                     string   `yaml:"type"`
		LoadBalancerSourceRanges []string `yaml:"loadBalancerSourceRanges"`
	} `yaml:"spec"`
}

type secretSpec struct {
	Type       string            `yaml:"type"`
	Data       map[string]string `yaml:"data"`
	StringData map[string]string `yaml:"stringData"`
}

type serviceAccountSpec struct {
	AutomountServiceAccountToken *bool `yaml:"automountServiceAccountToken"`
}

// ResourceGraph is every parsed manifest from one scan, indexed the way the
// RBAC and network rule families need to cross-reference resources (a
// RoleBinding needs its Role/ClusterRole's rules; a workload's "does a
// NetworkPolicy cover it" check needs every NetworkPolicy in its namespace).
type ResourceGraph struct {
	Manifests []Manifest

	// Namespace -> Role/ClusterRole name -> Manifest, split because
	// ClusterRole names are only unique cluster-wide but Role names are
	// only unique per-namespace — "" is the cluster scope key for
	// ClusterRoles.
	rolesByNamespace map[string]map[string]Manifest

	networkPoliciesByNamespace map[string][]Manifest
}

func newResourceGraph() *ResourceGraph {
	return &ResourceGraph{
		rolesByNamespace:           map[string]map[string]Manifest{},
		networkPoliciesByNamespace: map[string][]Manifest{},
	}
}

func (g *ResourceGraph) add(m Manifest) {
	g.Manifests = append(g.Manifests, m)

	if m.Role != nil {
		scope := m.Namespace
		if m.Kind == "ClusterRole" {
			scope = ""
		}
		if g.rolesByNamespace[scope] == nil {
			g.rolesByNamespace[scope] = map[string]Manifest{}
		}
		g.rolesByNamespace[scope][m.Name] = m
	}
	if m.NetworkPolicy != nil {
		g.networkPoliciesByNamespace[m.Namespace] = append(g.networkPoliciesByNamespace[m.Namespace], m)
	}
}

// roleFor resolves a RoleBinding's RoleRef to the Role/ClusterRole it
// points at. A ClusterRole can be referenced from any namespace (that's
// what makes cluster-admin-binding dangerous regardless of where the
// binding itself lives); a Role is only visible within its own namespace.
func (g *ResourceGraph) roleFor(bindingNamespace, refKind, refName string) (Manifest, bool) {
	if refKind == "ClusterRole" {
		m, ok := g.rolesByNamespace[""][refName]
		return m, ok
	}
	m, ok := g.rolesByNamespace[bindingNamespace][refName]
	return m, ok
}

// networkPoliciesFor returns every NetworkPolicy in namespace — the
// pod-selector matching NetworkPolicy semantics actually need (does a
// specific policy's selector match this specific pod's labels) is real but
// not implemented here; k8sscan.network.no-networkpolicy instead uses the
// coarser, still-accurate-per-the-module-spec signal "does this namespace
// have at least one NetworkPolicy at all."
func (g *ResourceGraph) networkPoliciesFor(namespace string) []Manifest {
	return g.networkPoliciesByNamespace[namespace]
}
