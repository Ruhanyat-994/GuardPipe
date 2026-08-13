package sandbox

import (
	"testing"

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

func TestNetworkNone_And_TargetOnly(t *testing.T) {
	none := NetworkNone()
	require.Equal(t, NetworkKindNone, none.Kind)

	target := NetworkTargetOnly("10.0.0.5", []int{80, 443})
	require.Equal(t, NetworkKindTargetOnly, target.Kind)
	require.Equal(t, "10.0.0.5", target.TargetIP)
	require.Equal(t, []int{80, 443}, target.TargetPorts)
}
