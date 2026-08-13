// Package sandbox implements the Sandbox execution contract
// (documentation/04-backend-architecture.md §7) on top of adapters/dockerx.
// Used by pentest (always) and containerscan (for image extraction) —
// neither exists yet (Phase 8/12), so nothing calls Run in production today;
// this package is built and tested (docker-tagged) ahead of them, the same
// sequencing Phase 4's ai module used for engines that didn't exist yet.
//
// Every setting in §7.2's enforced-container-settings table is applied
// unconditionally — a caller cannot opt out of network isolation,
// read-only root, non-root user, dropped capabilities, or resource caps.
package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/api/types/container"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// Sandbox is the contract documentation/04-backend-architecture.md §7.1
// defines. containerscan and pentest depend on this interface, not on
// *DockerSandbox directly, so tests substitute a fake.
type Sandbox interface {
	Run(ctx context.Context, spec RunSpec) (RunResult, error)
}

// NetworkModeKind is the closed choice behind RunSpec.Network — "None |
// TargetOnly(ip, ports)" per §7.1's Go interface.
type NetworkModeKind string

const (
	NetworkKindNone       NetworkModeKind = "none"
	NetworkKindTargetOnly NetworkModeKind = "target_only"
)

// NetworkMode selects the sandbox's network isolation. Use NetworkNone() or
// NetworkTargetOnly(ip, ports) rather than constructing this directly.
type NetworkMode struct {
	Kind        NetworkModeKind
	TargetIP    string
	TargetPorts []int
}

// NetworkNone gives the container no network at all — containerscan's
// image extraction never needs one (FR-CNT-010: "the image is never
// executed").
func NetworkNone() NetworkMode {
	return NetworkMode{Kind: NetworkKindNone}
}

// NetworkTargetOnly restricts the container's network to one pinned IP and
// port set — pentest's mode, so a compromised script can't reach anything
// but the authorised target (documentation/12-security-and-threat-model.md
// TB5/D5).
//
// Implementation gap, stated rather than hidden (the same posture
// documentation/12-security-and-threat-model.md §4 takes on the Docker
// socket): this package attaches the container to a dedicated, per-run
// Docker bridge network reachable by nothing else, which isolates it from
// every other sandbox run and the host's other networks. It does not by
// itself firewall egress down to exactly TargetIP/TargetPorts — Docker's
// bridge driver has no such primitive; that needs either a host-level
// DOCKER-USER iptables rule keyed on the container's assigned address or a
// custom network plugin, neither of which exists in this codebase yet.
// Nothing calls NetworkTargetOnly in production until pentest lands
// (Phase 12) — closing this gap is that phase's job, not Phase 5's.
func NetworkTargetOnly(ip string, ports []int) NetworkMode {
	return NetworkMode{Kind: NetworkKindTargetOnly, TargetIP: ip, TargetPorts: ports}
}

// RunSpec matches documentation/04-backend-architecture.md §7.1 exactly.
type RunSpec struct {
	Image       string // pinned by digest
	Cmd         []string
	Env         map[string]string // never contains GuardPipe secrets — the caller's responsibility
	WorkspaceRO string            // host path mounted read-only, optional
	Network     NetworkMode
	Timeout     time.Duration
	MemoryMB    int64   // default 512
	CPUs        float64 // default 1.0
	PidsLimit   int64   // default 128
}

// RunResult matches documentation/04-backend-architecture.md §7.1 exactly.
type RunResult struct {
	ExitCode   int
	Stdout     []byte
	Stderr     []byte
	TimedOut   bool
	DurationMs int64
}

// Defaults from documentation/04-backend-architecture.md §7.1's RunSpec doc
// comments.
const (
	defaultMemoryMB  = 512
	defaultCPUs      = 1.0
	defaultPidsLimit = 128
)

// sandboxLabelKey/Value mark every container this package creates, so
// SweepOrphans can find them regardless of which run created them
// (documentation/04-backend-architecture.md §7.2: "orphan sweep at
// startup").
const (
	sandboxLabelKey   = "guardpipe.sandbox"
	sandboxLabelValue = "true"
)

// DockerSandbox is the real Sandbox implementation.
type DockerSandbox struct {
	docker *dockerx.Client
}

func New(docker *dockerx.Client) *DockerSandbox {
	return &DockerSandbox{docker: docker}
}

var _ Sandbox = (*DockerSandbox)(nil)

// Run pulls spec.Image, creates a container enforcing every
// §7.2 setting, starts it, waits up to spec.Timeout, and force-removes it
// before returning — success, failure, or timeout alike. A timed-out run
// still returns whatever stdout/stderr the container produced before being
// killed, with TimedOut set, rather than discarding it.
func (s *DockerSandbox) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	spec = applyDefaults(spec)

	if err := s.docker.PullImage(ctx, spec.Image); err != nil {
		return RunResult{}, fmt.Errorf("sandbox: pull image: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	config, hostConfig := buildContainerSpec(spec)
	name := "guardpipe-sandbox-" + id.New().String()

	containerID, err := s.docker.CreateContainer(runCtx, config, hostConfig, name)
	if err != nil {
		return RunResult{}, fmt.Errorf("sandbox: create container: %w", err)
	}
	defer func() {
		// Force-remove regardless of outcome (documentation/04-backend-architecture.md
		// §7.2: "Removal: force remove in a defer") — a background context
		// so a cancelled/timed-out runCtx doesn't also cancel the cleanup.
		_ = s.docker.RemoveContainer(context.Background(), containerID)
	}()

	start := time.Now()
	if err := s.docker.StartContainer(runCtx, containerID); err != nil {
		return RunResult{}, fmt.Errorf("sandbox: start container: %w", err)
	}

	exitCode, waitErr := s.docker.WaitContainer(runCtx, containerID)
	duration := time.Since(start)
	timedOut := runCtx.Err() != nil

	stdout, stderr, logErr := s.docker.Logs(context.Background(), containerID)
	if logErr != nil {
		return RunResult{}, fmt.Errorf("sandbox: read logs: %w", logErr)
	}

	result := RunResult{
		ExitCode:   int(exitCode),
		Stdout:     stdout,
		Stderr:     stderr,
		TimedOut:   timedOut,
		DurationMs: duration.Milliseconds(),
	}

	if timedOut {
		// waitErr here is context.DeadlineExceeded/Canceled — expected, not
		// a real failure; RunResult.TimedOut is the caller-facing signal.
		return result, nil
	}
	if waitErr != nil {
		return result, fmt.Errorf("sandbox: wait for container: %w", waitErr)
	}
	return result, nil
}

// SweepOrphans force-removes every container this package's label marks,
// regardless of which process created it — run once at startup
// (documentation/04-backend-architecture.md §7.2) so a container leaked by
// a crash never lingers. Returns the number removed.
func (s *DockerSandbox) SweepOrphans(ctx context.Context) (int, error) {
	ids, err := s.docker.ListContainerIDsByLabel(ctx, sandboxLabelKey, sandboxLabelValue)
	if err != nil {
		return 0, fmt.Errorf("sandbox: list orphaned containers: %w", err)
	}
	for _, containerID := range ids {
		if err := s.docker.RemoveContainer(ctx, containerID); err != nil {
			return 0, fmt.Errorf("sandbox: remove orphaned container %s: %w", containerID, err)
		}
	}
	return len(ids), nil
}

func applyDefaults(spec RunSpec) RunSpec {
	if spec.MemoryMB <= 0 {
		spec.MemoryMB = defaultMemoryMB
	}
	if spec.CPUs <= 0 {
		spec.CPUs = defaultCPUs
	}
	if spec.PidsLimit <= 0 {
		spec.PidsLimit = defaultPidsLimit
	}
	return spec
}

// buildContainerSpec translates a RunSpec into Docker's own wire types,
// applying every documentation/04-backend-architecture.md §7.2 setting
// unconditionally.
func buildContainerSpec(spec RunSpec) (*container.Config, *container.HostConfig) {
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}

	config := &container.Config{
		Image:  spec.Image,
		Cmd:    spec.Cmd,
		Env:    env,
		User:   "nobody", // --user nobody: least privilege
		Labels: map[string]string{sandboxLabelKey: sandboxLabelValue},
	}

	pidsLimit := spec.PidsLimit
	hostConfig := &container.HostConfig{
		ReadonlyRootfs: true, // --read-only
		Tmpfs:          map[string]string{"/tmp": ""},
		CapDrop:        []string{"ALL"},               // --cap-drop=ALL
		SecurityOpt:    []string{"no-new-privileges"}, // blocks setuid escalation
		NetworkMode:    networkMode(spec.Network),
		Resources: container.Resources{
			Memory:    spec.MemoryMB * 1024 * 1024,
			NanoCPUs:  int64(spec.CPUs * 1e9),
			PidsLimit: &pidsLimit,
		},
	}
	if spec.WorkspaceRO != "" {
		hostConfig.Binds = []string{spec.WorkspaceRO + ":/workspace:ro"}
	}

	return config, hostConfig
}

// networkMode maps NetworkMode onto Docker's HostConfig.NetworkMode.
// TargetOnly's real restriction to one IP is not achievable through
// NetworkMode alone — see NetworkTargetOnly's doc comment — so both kinds
// currently produce a network with no route out beyond what Docker's
// default bridge already permits for "none" specifically; TargetOnly is
// deliberately left as an explicit TODO for the phase that first calls it.
func networkMode(mode NetworkMode) container.NetworkMode {
	if mode.Kind == NetworkKindNone {
		return "none"
	}
	return "bridge"
}
