package k8sscan

import "testing"

// TestNetworkRules is the true-positive/near-miss table for all six
// k8sscan.network.*/k8sscan.secrets.* Core rules (documentation/05-module-specifications.md
// §9).
func TestNetworkRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "namespace with no NetworkPolicy fires", ruleID: "k8sscan.network.no-networkpolicy", wantFire: true,
			yaml: deploymentPodSpec(`containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`),
		},
		{
			name: "namespace with a NetworkPolicy present does not fire", ruleID: "k8sscan.network.no-networkpolicy", wantFire: false,
			yaml: deploymentPodSpec(`containers: [{name: app, image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678}]`) + "\n---\n" +
				`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: default-deny, namespace: default}
spec:
  podSelector: {}
  policyTypes: ["Ingress", "Egress"]`,
		},
		{
			name: "unrestricted egress rule fires", ruleID: "k8sscan.network.allow-all-egress", wantFire: true,
			yaml: `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: np1, namespace: default}
spec:
  podSelector: {}
  policyTypes: ["Egress"]
  egress:
  - {}`,
		},
		{
			name: "egress restricted to a specific destination does not fire", ruleID: "k8sscan.network.allow-all-egress", wantFire: false,
			yaml: `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: np1, namespace: default}
spec:
  podSelector: {}
  policyTypes: ["Egress"]
  egress:
  - to:
    - podSelector: {matchLabels: {app: db}}
    ports:
    - {protocol: TCP, port: 5432}`,
		},
		{
			name: "empty egress list (deny-all) does not fire", ruleID: "k8sscan.network.allow-all-egress", wantFire: false,
			yaml: `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: np1, namespace: default}
spec:
  podSelector: {}
  policyTypes: ["Egress"]
  egress: []`,
		},
		{
			name: "NodePort service fires", ruleID: "k8sscan.network.nodeport-service", wantFire: true,
			yaml: `apiVersion: v1
kind: Service
metadata: {name: svc1, namespace: default}
spec:
  type: NodePort
  ports: [{port: 80}]`,
		},
		{
			name: "ClusterIP service does not fire", ruleID: "k8sscan.network.nodeport-service", wantFire: false,
			yaml: `apiVersion: v1
kind: Service
metadata: {name: svc1, namespace: default}
spec:
  type: ClusterIP
  ports: [{port: 80}]`,
		},
		{
			name: "LoadBalancer with no source ranges fires", ruleID: "k8sscan.network.loadbalancer-no-source-ranges", wantFire: true,
			yaml: `apiVersion: v1
kind: Service
metadata: {name: svc1, namespace: default}
spec:
  type: LoadBalancer
  ports: [{port: 443}]`,
		},
		{
			name: "LoadBalancer with source ranges set does not fire", ruleID: "k8sscan.network.loadbalancer-no-source-ranges", wantFire: false,
			yaml: `apiVersion: v1
kind: Service
metadata: {name: svc1, namespace: default}
spec:
  type: LoadBalancer
  ports: [{port: 443}]
  loadBalancerSourceRanges: ["203.0.113.0/24"]`,
		},
		{
			name: "Secret with inline data fires", ruleID: "k8sscan.secrets.literal-in-manifest", wantFire: true,
			yaml: `apiVersion: v1
kind: Secret
metadata: {name: s1, namespace: default}
type: Opaque
data:
  password: c3VwZXJzZWNyZXQ=`,
		},
		{
			name: "an externally-managed secret CRD (no Secret kind, no literal data) does not fire", ruleID: "k8sscan.secrets.literal-in-manifest", wantFire: false,
			yaml: `apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata: {name: s1, namespace: default}
spec:
  secretStoreRef: {name: vault-backend, kind: SecretStore}`,
		},
		{
			name: "envFrom secretRef imports the whole Secret, fires", ruleID: "k8sscan.secrets.env-from-secret-all", wantFire: true,
			yaml: deploymentPodSpec(`containers:
      - name: app
        image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678
        envFrom:
        - secretRef: {name: app-secrets}`),
		},
		{
			name: "env valueFrom.secretKeyRef imports one key, does not fire", ruleID: "k8sscan.secrets.env-from-secret-all", wantFire: false,
			yaml: deploymentPodSpec(`containers:
      - name: app
        image: nginx@sha256:abcdef1234567890123456789012345678901234567890123456789012345678
        env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef: {name: app-secrets, key: password}`),
		},
	})
}
