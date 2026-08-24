package sandbox

import (
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestApplyDefaults_FillsZeroValues(t *testing.T) {
	spec := applyDefaults(RunSpec{})
	require.Equal(t, int64(defaultMemoryMB), spec.MemoryMB)
	require.Equal(t, float64(defaultCPUs), spec.CPUs)
	require.Equal(t, int64(defaultPidsLimit), spec.PidsLimit)
}

func TestApplyDefaults_PreservesExplicitValues(t *testing.T) {
	spec := applyDefaults(RunSpec{MemoryMB: 1024, CPUs: 2.0, PidsLimit: 64})
	require.Equal(t, int64(1024), spec.MemoryMB)
	require.Equal(t, 2.0, spec.CPUs)
	require.Equal(t, int64(64), spec.PidsLimit)
}

func TestBuildContainerSpec_EnforcesSecuritySettings(t *testing.T) {
	spec := applyDefaults(RunSpec{Image: "alpine:3.20", Cmd: []string{"echo", "hi"}, Network: NetworkNone()})
	config, hostConfig := buildContainerSpec(spec)

	require.Equal(t, "nobody", config.User, "--user nobody: least privilege")
	require.Equal(t, sandboxLabelValue, config.Labels[sandboxLabelKey])

	require.True(t, hostConfig.ReadonlyRootfs, "--read-only")
	require.Contains(t, hostConfig.Tmpfs, "/tmp")
	require.Equal(t, []string{"ALL"}, hostConfig.CapDrop, "--cap-drop=ALL")
	require.Contains(t, hostConfig.SecurityOpt, "no-new-privileges")
	require.Equal(t, "none", string(hostConfig.NetworkMode))
	require.Equal(t, int64(defaultMemoryMB*1024*1024), hostConfig.Memory)
	require.NotNil(t, hostConfig.PidsLimit)
	require.Equal(t, int64(defaultPidsLimit), *hostConfig.PidsLimit)
}

func TestBuildContainerSpec_NeverExposesDockerSocket(t *testing.T) {
	spec := applyDefaults(RunSpec{Image: "alpine:3.20", Network: NetworkNone()})
	_, hostConfig := buildContainerSpec(spec)

	for _, bind := range hostConfig.Binds {
		require.NotContains(t, bind, "docker.sock", "the sandbox must never receive the Docker socket (documentation/04-backend-architecture.md §7.2)")
	}
}

func TestBuildContainerSpec_MountsWorkspaceReadOnly(t *testing.T) {
	spec := applyDefaults(RunSpec{Image: "alpine:3.20", WorkspaceRO: "/host/checkout", Network: NetworkNone()})
	_, hostConfig := buildContainerSpec(spec)

	require.Contains(t, hostConfig.Binds, "/host/checkout:/workspace:ro")
}

func TestBuildContainerSpec_EnvIsFormattedKeyEqualsValue(t *testing.T) {
	spec := applyDefaults(RunSpec{Image: "alpine:3.20", Env: map[string]string{"TARGET": "example.com"}, Network: NetworkNone()})
	config, _ := buildContainerSpec(spec)

	require.Contains(t, config.Env, "TARGET=example.com")
}

// TestBuildContainerSpec_TargetOnlyAddsFirewallCapabilities is the
// true-positive half: only a TargetOnly run gets the NET_ADMIN/NET_RAW
// exception and its Cmd rewritten to self-firewall first.
func TestBuildContainerSpec_TargetOnlyAddsFirewallCapabilities(t *testing.T) {
	spec := applyDefaults(RunSpec{
		Image: "alpine:3.20", Cmd: []string{"nmap", "-p-", "203.0.113.10"},
		Network: NetworkTargetOnly([]string{"203.0.113.10"}, []int{80, 443}),
	})
	config, hostConfig := buildContainerSpec(spec)

	require.Equal(t, []string{"ALL"}, hostConfig.CapDrop, "CapDrop:ALL is never relaxed, only narrowly added back")
	require.ElementsMatch(t, []string{"NET_ADMIN", "NET_RAW", "SETUID", "SETGID"}, hostConfig.CapAdd, "SETUID/SETGID are what let su-exec actually drop root to nobody")
	// The container briefly starts as root (User left unset/empty) so the
	// iptables bootstrap can run at all — Alpine's nftables-backed iptables
	// refuses the same operation from a non-root process even with
	// NET_ADMIN/NET_RAW granted via file capabilities (confirmed against
	// the real sandbox image). firewalledCmd's own su-exec drop is what
	// actually keeps the real tool script off of root, asserted below.
	require.Equal(t, "", config.User, "TargetOnly starts as root only long enough to firewall, then su-execs down")
	require.Contains(t, hostConfig.Tmpfs, "/run", "iptables' lock file needs a writable /run under ReadonlyRootfs")

	require.Equal(t, []string{"/bin/sh", "-c"}, config.Cmd[:2])
	bootstrap := config.Cmd[2]
	require.Contains(t, bootstrap, "iptables -P OUTPUT DROP")
	require.Contains(t, bootstrap, "-d 203.0.113.10 -j ACCEPT")
	require.Contains(t, bootstrap, "--dport 53 -j ACCEPT", "DNS must stay reachable — every tool resolves TARGET_HOST for TLS/SNI/vhost routing, confirmed the hard way against a real CDN-fronted target")
	require.Contains(t, bootstrap, `exec su-exec nobody env HOME=/tmp "$@"`, "the real tool script must never run as root, and must get a writable HOME — su-exec resets HOME to nobody's own passwd entry (\"/\"), which broke any tool that writes a per-user config/cache on first run")
	require.Equal(t, []string{"nmap", "-p-", "203.0.113.10"}, config.Cmd[4:], "the original Cmd must still run, unmodified, as the trailing exec args")
}

// TestBuildContainerSpec_NetworkNoneNeverGetsFirewallCapabilities is the
// near-miss complement — NetworkNone (containerscan's image-extraction use)
// must not fire the TargetOnly-only exception just because CapAdd exists as
// a mechanism now.
func TestBuildContainerSpec_NetworkNoneNeverGetsFirewallCapabilities(t *testing.T) {
	spec := applyDefaults(RunSpec{Image: "alpine:3.20", Cmd: []string{"echo", "hi"}, Network: NetworkNone()})
	config, hostConfig := buildContainerSpec(spec)

	require.Nil(t, hostConfig.CapAdd)
	require.Equal(t, []string{"echo", "hi"}, config.Cmd, "Cmd must pass through unmodified when there is no firewall to bootstrap")
}

// TestFirewalledCmd_EmptyTargetIPFailsClosed is the near-miss for
// firewalledCmd's own doc comment: an empty targetIP must never fall back
// to allowing everything.
func TestFirewalledCmd_EmptyTargetIPFailsClosed(t *testing.T) {
	cmd := firewalledCmd(nil, []string{"true"})
	bootstrap := cmd[2]
	require.NotContains(t, bootstrap, "-j ACCEPT -d", "must not contain a malformed/empty-destination ACCEPT rule")
	require.Contains(t, bootstrap, "iptables -P OUTPUT DROP")
	require.NotContains(t, bootstrap, " -d  -j ACCEPT", "an empty target must never produce an ACCEPT rule with no destination")
	require.Contains(t, bootstrap, "exec su-exec nobody", "even the fail-closed path must still drop to a non-root user before exec")
	require.Contains(t, bootstrap, "--dport 53 -j ACCEPT", "DNS stays reachable even on the fail-closed path — it's an independent, deliberate exception, not tied to a valid target being present")
}

func TestBuildContainerSpec_ScriptMountUsesNamedVolumeSubpath(t *testing.T) {
	spec := applyDefaults(RunSpec{
		Image:   "alpine:3.20",
		Network: NetworkNone(),
		ScriptMount: &VolumeMount{
			VolumeName: "guardpipe-workspace", Subpath: "pentest-scripts", Target: "/pentest-scripts",
		},
	})
	_, hostConfig := buildContainerSpec(spec)

	require.Len(t, hostConfig.Mounts, 1)
	m := hostConfig.Mounts[0]
	require.Equal(t, "guardpipe-workspace", m.Source)
	require.Equal(t, "/pentest-scripts", m.Target)
	require.True(t, m.ReadOnly)
	require.NotNil(t, m.VolumeOptions)
	require.Equal(t, "pentest-scripts", m.VolumeOptions.Subpath)
}

func TestNetworkNone_And_TargetOnly(t *testing.T) {
	none := NetworkNone()
	require.Equal(t, NetworkKindNone, none.Kind)

	target := NetworkTargetOnly([]string{"10.0.0.5"}, []int{80, 443})
	require.Equal(t, NetworkKindTargetOnly, target.Kind)
	require.Equal(t, []string{"10.0.0.5"}, target.TargetIPs)
	require.Equal(t, []int{80, 443}, target.TargetPorts)
}

func TestNetworkOpenEgress_Kind(t *testing.T) {
	open := NetworkOpenEgress()
	require.Equal(t, NetworkKindOpenEgress, open.Kind)
}

// TestBuildContainerSpec_OpenEgressStillLockedDownExceptNetwork is
// NetworkOpenEgress's own contract: bridge networking (unlike NetworkNone),
// but every other §7.2 container setting stays exactly as strict as
// NetworkNone/TargetOnly get — no firewall capability escalation, runs as
// nobody immediately (no root-then-drop needed, since there's no iptables
// bootstrap to run), Cmd passes through unmodified.
func TestBuildContainerSpec_OpenEgressStillLockedDownExceptNetwork(t *testing.T) {
	spec := applyDefaults(RunSpec{
		Image: "alpine:3.20", Cmd: []string{"subfinder", "-d", "example.com"},
		Network: NetworkOpenEgress(),
	})
	config, hostConfig := buildContainerSpec(spec)

	require.Equal(t, container.NetworkMode("bridge"), hostConfig.NetworkMode, "open egress still needs a real network attached, unlike NetworkNone")
	require.Equal(t, []string{"ALL"}, hostConfig.CapDrop)
	require.Nil(t, hostConfig.CapAdd, "no firewall to bootstrap means no capability exception is needed")
	require.Equal(t, "nobody", config.User, "runs as nobody immediately — no root-then-drop needed without an iptables bootstrap")
	require.Equal(t, []string{"subfinder", "-d", "example.com"}, config.Cmd, "Cmd must pass through unmodified, same as NetworkNone")
	require.True(t, hostConfig.ReadonlyRootfs)
}
