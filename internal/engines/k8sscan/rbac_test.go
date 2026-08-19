package k8sscan

import "testing"

// TestRBACRules is the true-positive/near-miss table for all ten
// k8sscan.rbac.* Core rules (documentation/05-module-specifications.md §9)
// — the near-miss half is what proves a *legitimately scoped* grant stays
// quiet, not just that an obviously-safe empty manifest does.
func TestRBACRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "wildcard verbs fires", ruleID: "k8sscan.rbac.wildcard-verbs", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["*"]`,
		},
		{
			name: "scoped verbs do not fire", ruleID: "k8sscan.rbac.wildcard-verbs", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]`,
		},
		{
			name: "wildcard resources fires", ruleID: "k8sscan.rbac.wildcard-resources", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["*"]
  verbs: ["get"]`,
		},
		{
			name: "scoped resources do not fire", ruleID: "k8sscan.rbac.wildcard-resources", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get"]`,
		},
		{
			name: "wildcard apiGroups fires", ruleID: "k8sscan.rbac.wildcard-apigroups", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: ["*"]
  resources: ["pods"]
  verbs: ["get"]`,
		},
		{
			name: "scoped apiGroups (core, explicit empty string) does not fire", ruleID: "k8sscan.rbac.wildcard-apigroups", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get"]`,
		},
		{
			name: "ClusterRole with cluster-wide secrets read fires", ruleID: "k8sscan.rbac.secrets-read-cluster-wide", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: cr1}
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]`,
		},
		{
			name: "namespaced Role with the same secrets read does not fire (not cluster-wide)", ruleID: "k8sscan.rbac.secrets-read-cluster-wide", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]`,
		},
		{
			name: "pod create fires", ruleID: "k8sscan.rbac.pod-create", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["create"]`,
		},
		{
			name: "pod read-only does not fire", ruleID: "k8sscan.rbac.pod-create", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]`,
		},
		{
			name: "escalate on roles fires", ruleID: "k8sscan.rbac.escalate-bind", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: cr1}
rules:
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles"]
  verbs: ["escalate"]`,
		},
		{
			name: "read-only roles access does not fire", ruleID: "k8sscan.rbac.escalate-bind", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: cr1}
rules:
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles"]
  verbs: ["get", "list"]`,
		},
		{
			name: "impersonate fires", ruleID: "k8sscan.rbac.impersonate", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: cr1}
rules:
- apiGroups: [""]
  resources: ["users"]
  verbs: ["impersonate"]`,
		},
		{
			name: "no impersonate verb does not fire", ruleID: "k8sscan.rbac.impersonate", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: cr1}
rules:
- apiGroups: [""]
  resources: ["users"]
  verbs: ["get"]`,
		},
		{
			name: "pod exec fires", ruleID: "k8sscan.rbac.exec-attach", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods/exec"]
  verbs: ["create"]`,
		},
		{
			name: "reading pod logs does not fire", ruleID: "k8sscan.rbac.exec-attach", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r1, namespace: default}
rules:
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]`,
		},
		{
			name: "binding to cluster-admin fires", ruleID: "k8sscan.rbac.cluster-admin-binding", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: b1}
roleRef: {kind: ClusterRole, name: cluster-admin}
subjects:
- {kind: ServiceAccount, name: app-sa, namespace: default}`,
		},
		{
			name: "binding to a narrow built-in role does not fire", ruleID: "k8sscan.rbac.cluster-admin-binding", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: b1}
roleRef: {kind: ClusterRole, name: view}
subjects:
- {kind: ServiceAccount, name: app-sa, namespace: default}`,
		},
		{
			name: "binding the default ServiceAccount fires", ruleID: "k8sscan.rbac.default-sa-bound", wantFire: true,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: {name: b1, namespace: default}
roleRef: {kind: Role, name: r1}
subjects:
- {kind: ServiceAccount, name: default, namespace: default}`,
		},
		{
			name: "binding a dedicated ServiceAccount does not fire", ruleID: "k8sscan.rbac.default-sa-bound", wantFire: false,
			yaml: `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata: {name: b1, namespace: default}
roleRef: {kind: Role, name: r1}
subjects:
- {kind: ServiceAccount, name: app-dedicated-sa, namespace: default}`,
		},
	})
}
