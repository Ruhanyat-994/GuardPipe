// Package sandbox implements the Sandbox execution contract
// (documentation/04-backend-architecture.md §7) on top of adapters/dockerx.
// Used by pentest (always) and containerscan (for image extraction) —
// neither exists yet (Phase 8/12), so nothing calls Run in production today;
// this package is built and tested (docker-tagged) ahead of them, the same
// sequencing Phase 4's ai module used for engines that didn't exist yet.
//
// Every setting in §7.2's enforced-container-settings table is applied
// unconditionally — a caller cannot opt out of network isolation,
// read-only root, non-root user, resource caps, or (with one narrow,
// documented exception — NET_ADMIN/NET_RAW, only for NetworkTargetOnly,
// only to let the container firewall its own egress, see buildContainerSpec)
// dropped capabilities.
package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

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
	NetworkKindOpenEgress NetworkModeKind = "open_egress"
)

// NetworkMode selects the sandbox's network isolation. Use NetworkNone() or
// NetworkTargetOnly(ips, ports) rather than constructing this directly.
type NetworkMode struct {
	Kind        NetworkModeKind
	TargetIPs   []string
	TargetPorts []int
}

// NetworkNone gives the container no network at all — containerscan's
// image extraction never needs one (FR-CNT-010: "the image is never
// executed").
func NetworkNone() NetworkMode {
	return NetworkMode{Kind: NetworkKindNone}
}

// NetworkTargetOnly restricts the container's network to the target's
// pinned IPs — pentest's mode, so a compromised script can't reach anything
// but the authorised target (documentation/12-security-and-threat-model.md
// TB5/D5). Takes every IP validate.ResolveTarget pinned at registration
// time, not just one: confirmed against a real CDN-fronted target
// (Vercel-hosted) that a single query's A records aren't the whole story —
// individual tool invocations, spread over a multi-minute scan, each do
// their own DNS lookup (needed for TLS SNI/Host-header vhost routing, not
// optional) and can legitimately land on a different address within the
// same provider's edge pool. Allowing the full originally-pinned set (still
// a bounded, known list — never "any IP") tolerates that rotation without
// abandoning the actual security boundary; `reverifyTarget`
// (engines/pentest/engine.go) is the layer that decides whether a resolved
// set has drifted enough to be genuine DNS-rebinding, not this firewall.
//
// Closed in Phase 12 Pass 2 (previously an acknowledged, stated gap: Docker's
// bridge driver alone has no per-destination egress primitive, and a
// host-level DOCKER-USER iptables rule isn't reachable from inside this
// process's own container). The fix needs no host access at all: the
// container firewalls its *own* egress, inside its own network namespace,
// before running the caller's Cmd — see buildContainerSpec's CapAdd/Cmd
// rewrite below for TargetOnly. TargetPorts is accepted for callers that
// want to record intent, but the actual firewall rule is IP-only (not
// IP:port), matching documentation/05-module-specifications.md §12's own
// "Sandbox network permits only the pinned IP" wording — Phase 1's own
// port-discovery scripts need to reach the target on ports nobody has
// enumerated yet, so a port-level restriction would break exactly the
// recon step this whole engine starts with.
func NetworkTargetOnly(ips []string, ports []int) NetworkMode {
	return NetworkMode{Kind: NetworkKindTargetOnly, TargetIPs: ips, TargetPorts: ports}
}

// NetworkOpenEgress gives the container full outbound internet access — no
// per-destination firewall at all — for the one class of pentest script that
// doesn't talk to the attested target in the first place: passive subdomain/
// asset enumeration (`subfinder`, `amass enum -passive`), which queries
// third-party public sources (certificate-transparency logs, DNS
// aggregators) *about* the target rather than sending it a single packet.
// `NetworkTargetOnly`'s firewall can't serve this case — that mode's whole
// design point is "reach only the one host the attestation actually covers,"
// and these tools structurally need to reach dozens of unrelated public
// hosts whose addresses aren't known in advance and change as each tool's
// own source list evolves; hand-maintaining an allowlist of OSINT-provider
// hostnames would be exactly the kind of fragile, silently-stale mapping
// documentation/12-security-and-threat-model.md's threat model warns against
// elsewhere. This is not a weaker container, only a different network: every
// other §7.2 setting (read-only rootfs, `--cap-drop=ALL`, non-root, resource
// caps, time limit) still applies unconditionally — see buildContainerSpec,
// which only special-cases capabilities/user for NetworkKindTargetOnly, not
// this kind. The actual scope boundary this relies on lives one layer up, in
// engines/pentest: BUILD_GUIDE.md Phase 12's Phase 7 "hard rule, not a
// suggestion" — anything an open-egress script discovers is reported as an
// informational finding only, never fed back into this or any later phase's
// own target list. A script run under this mode must never be handed
// anything secret (it can freely exfiltrate whatever env vars/mounts it can
// see) — today that's just TARGET_HOST/RATE/PHASE_BUDGET_SECONDS, no
// credential ever reaches a pentest sandbox container of any network kind.
func NetworkOpenEgress() NetworkMode {
	return NetworkMode{Kind: NetworkKindOpenEgress}
}

// VolumeMount mounts a named Docker volume (optionally at a subpath) into
// the sandbox container, read-only — the same Docker-outside-of-Docker
// mechanism adapters/trivy and adapters/sonarqube already use for their own
// workspace mounts (a plain host bind path doesn't resolve correctly when
// the caller is itself containerized; see their own doc comments). Used by
// pentest to deliver its go:embed'd scripts, which are written into this
// volume once at startup rather than baked into the sandbox image, so
// script logic ships and versions with the app binary.
type VolumeMount struct {
	VolumeName string
	Subpath    string
	Target     string // mount point inside the sandbox container, e.g. "/pentest-scripts"
}

// RunSpec matches documentation/04-backend-architecture.md §7.1, plus
// ScriptMount (Phase 12 Pass 2 — see VolumeMount's own doc comment).
type RunSpec struct {
	Image       string // pinned by digest, or a locally-built-only tag — see ImageExists' doc comment on Run
	Cmd         []string
	Env         map[string]string // never contains GuardPipe secrets — the caller's responsibility
	WorkspaceRO string            // host path mounted read-only, optional
	ScriptMount *VolumeMount      // named-volume mount, optional — mutually exclusive with WorkspaceRO in practice, never both
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

	// A locally-built-only image (the pentest sandbox image, built via
	// `docker compose build pentest-sandbox`, never pushed anywhere) has
	// nothing to pull from any registry — PullImage would fail every time.
	// Every other caller (a real pinned tag/digest) still pulls exactly as
	// before; this only skips the pull when the image is already present.
	exists, err := s.docker.ImageExists(ctx, spec.Image)
	if err != nil {
		return RunResult{}, fmt.Errorf("sandbox: check local image: %w", err)
	}
	if !exists {
		if err := s.docker.PullImage(ctx, spec.Image); err != nil {
			return RunResult{}, fmt.Errorf("sandbox: pull image: %w", err)
		}
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

// RunningSandboxCount satisfies admin.SandboxHealthReader for
// `GET /admin/system-health` (BUILD_GUIDE.md Phase 14) — a real count of
// this package's own containers currently alive, not a fabricated number.
// Structurally implements admin.SandboxHealthReader without importing
// modules/admin (an adapter has no business knowing about a platform
// module — the interface is satisfied by shape, matched up in
// cmd/guardpipe/main.go where both packages are already in scope).
func (s *DockerSandbox) RunningSandboxCount(ctx context.Context) (int, error) {
	ids, err := s.docker.ListContainerIDsByLabel(ctx, sandboxLabelKey, sandboxLabelValue)
	if err != nil {
		return 0, fmt.Errorf("sandbox: count running containers: %w", err)
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
// unconditionally, plus the one narrow, justified exception described below.
func buildContainerSpec(spec RunSpec) (*container.Config, *container.HostConfig) {
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}

	cmd := spec.Cmd
	user := "nobody" // --user nobody: least privilege, the default for every mode
	var capAdd []string
	if spec.Network.Kind == NetworkKindTargetOnly {
		// The one narrow, justified exception to this package's own
		// CapDrop:ALL/non-root defaults — same category of deliberate
		// carve-out as adapters/trivy/adapters/sonarqube's network
		// exception (see their own doc comments), scoped as tightly as the
		// job it does. The container starts as root (user left unset, the
		// pentest sandbox image's default) only long enough for
		// firewalledCmd's iptables bootstrap to run — file-capability
		// (setcap) delegation to a non-root user was tried first and
		// rejected: Alpine's nftables-backed iptables refuses to touch its
		// rule set from an unprivileged process even with
		// NET_ADMIN/NET_RAW granted via setcap (confirmed against the real
		// image — "Permission denied (you must be root)"), the same
		// operation succeeding immediately as root with identical
		// capabilities. firewalledCmd drops to "nobody" via su-exec itself
		// before the real tool command ever runs, so no tool script is
		// ever reached as root. See firewalledCmd's doc comment for the
		// mechanism this closes documentation/12-security-and-threat-model.md
		// TB5/D5 and the "network: target IP only" requirement with.
		// SETUID/SETGID are also needed here, easy to miss: CapDrop:ALL
		// means UID 0 in this container is "root" in name only — su-exec's
		// own setuid()/setgroups() calls to become "nobody" fail with
		// EPERM without them (confirmed against the real image), same as
		// any other capability this package grants only as narrowly as the
		// job requires.
		user = ""
		capAdd = []string{"NET_ADMIN", "NET_RAW", "SETUID", "SETGID"}
		cmd = firewalledCmd(spec.Network.TargetIPs, spec.Cmd)
	}

	config := &container.Config{
		Image:  spec.Image,
		Cmd:    cmd,
		Env:    env,
		User:   user,
		Labels: map[string]string{sandboxLabelKey: sandboxLabelValue},
	}

	pidsLimit := spec.PidsLimit
	hostConfig := &container.HostConfig{
		ReadonlyRootfs: true, // --read-only
		// /run is writable too, not just /tmp: iptables' lock file
		// (/run/xtables.lock) needs a writable /run to acquire under
		// ReadonlyRootfs — harmless for every other caller, which never
		// touches /run at all.
		Tmpfs:       map[string]string{"/tmp": "", "/run": ""},
		CapDrop:     []string{"ALL"},               // --cap-drop=ALL
		CapAdd:      capAdd,                        // nil for every mode except TargetOnly
		SecurityOpt: []string{"no-new-privileges"}, // blocks setuid escalation
		NetworkMode: networkMode(spec.Network),
		Resources: container.Resources{
			Memory:    spec.MemoryMB * 1024 * 1024,
			NanoCPUs:  int64(spec.CPUs * 1e9),
			PidsLimit: &pidsLimit,
		},
	}
	if spec.WorkspaceRO != "" {
		hostConfig.Binds = []string{spec.WorkspaceRO + ":/workspace:ro"}
	}
	if spec.ScriptMount != nil {
		hostConfig.Mounts = []mount.Mount{{
			Type:     mount.TypeVolume,
			Source:   spec.ScriptMount.VolumeName,
			Target:   spec.ScriptMount.Target,
			ReadOnly: true,
			VolumeOptions: &mount.VolumeOptions{
				Subpath: spec.ScriptMount.Subpath,
			},
		}}
	}

	return config, hostConfig
}

// firewalledCmd wraps cmd so the container firewalls its own egress down to
// targetIP — entirely inside its own network namespace, no host-level
// Docker Desktop VM access needed — before running it. Bootstrap: default-
// DROP the OUTPUT chain, allow loopback (needed for anything that talks to
// itself, e.g. testssl.sh's internal helpers), allow DNS (port 53, UDP+TCP,
// to any destination — see below for why this can't be narrowed to
// targetIP), allow only targetIP for everything else, then drop from root
// to "nobody" (su-exec, baked into the pentest sandbox image) and exec the
// real command — no tool script ever runs as root, even though the
// container briefly starts as root to set these rules (see
// buildContainerSpec's own doc comment for why root-then-drop replaced an
// earlier setcap-based attempt). ip6tables gets the same default-DROP with
// no exceptions (IPv6 egress is never needed — every target is pinned by
// IPv4, and Docker's default bridge has no IPv6 route out anyway; this just
// closes the path defensively), tolerating its absence with `|| true` since
// not every iptables package ships it.
//
// Why DNS can't just be narrowed to targetIP: every pentest script builds
// its request against TARGET_HOST (the hostname), not the pinned IP — this
// is required, not a convenience, for any TLS/SNI or Host-header-routed
// target (e.g. a site fronted by Vercel/Cloudflare/any shared-IP CDN, where
// connecting by bare IP reaches the wrong backend or fails the TLS
// handshake outright). Confirmed the hard way: an earlier version of this
// firewall allowed only targetIP and every curl/ffuf/nuclei/etc. call
// silently returned nothing, because DNS resolution of TARGET_HOST itself
// was being dropped before any tool-level request could even be attempted.
// The resolver the container actually reaches depends on what Docker wrote
// into /etc/resolv.conf (not otherwise controlled here), so the ACCEPT rule
// targets the port, not a specific resolver IP. This is a narrow,
// low-risk exception (DNS is a read-only, small-response protocol — it
// cannot serve target content or arbitrary data back to the scanner) next
// to the still-absolute rule that no other host, on any other port, is
// ever reachable.
//
// Why the originally-pinned IPs alone still aren't enough, and what closes
// that gap: confirmed against a real CDN-fronted target (Vercel) that its
// resolvable address pool is wider than any single lookup captures — a
// tool's own DNS query minutes into the same scan can legitimately return
// an address that was never in the pinned set at all, not just a different
// member of it. The bootstrap re-resolves TARGET_HOST itself (nslookup,
// already permitted by the DNS exception above) and ACCEPTs whatever it
// gets back, *in addition to* the originally-pinned IPs — but only after
// checking each freshly-resolved address isn't private/loopback/link-local/
// metadata (the same range list validate.isBlockedRange rejects at
// registration), so a target that rebinds DNS to an internal address
// mid-scan still can't walk this exception into an SSRF: reverifyTarget
// (engines/pentest/engine.go) is the layer that decides whether that kind
// of drift means the whole job aborts as suspected rebinding; this
// firewall's job is narrower — never let a script reach a non-public,
// non-attested address, no matter which layer's check is what actually
// catches a given attempt.
//
// Known, accepted residual gap (not silently swept under the rug):
// reverifyTarget only re-resolves once, at the start of Run() — it can't see
// a rebind that happens *after* that check but *before* a later phase's own
// script runs, potentially minutes later in a multi-phase scan. This
// bootstrap's per-invocation re-resolution would trust a newly-rebound
// *public* IP in that narrow window (a rebind to a private/internal address
// is still caught, per is_public_ipv4 above). Closing that fully would mean
// either re-running reverifyTarget's full check before every single script
// invocation (a real design change, not a one-line fix — TODO for a later
// pass) or giving up CDN/shared-IP-target support entirely, which was the
// blocking bug this whole mechanism exists to fix. Accepted for now because
// the attack this narrows to (an attacker controlling the attested target's
// own authoritative DNS, specifically timing a rebind to their own public
// infrastructure mid-scan) already implies a privileged position against
// that target, not an arbitrary third party.
//
// Fails closed by design: an empty targetIP (should never happen — callers
// only reach here via NetworkTargetOnly, which requires one) produces a
// bootstrap with no target ACCEPT rule at all (DNS is still permitted, per
// above), meaning every non-DNS destination stays dropped rather than
// silently allowing full egress.
func firewalledCmd(targetIPs []string, cmd []string) []string {
	bootstrap := "iptables -P OUTPUT DROP && iptables -A OUTPUT -o lo -j ACCEPT"
	bootstrap += " && iptables -A OUTPUT -p udp --dport 53 -j ACCEPT && iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT"
	for _, ip := range targetIPs {
		if ip == "" {
			continue
		}
		bootstrap += fmt.Sprintf(" && iptables -A OUTPUT -d %s -j ACCEPT", ip)
	}
	// is_public_ipv4 rejects anything in 10/8, 172.16/12, 192.168/16,
	// 127/8, 169.254/16 (incl. 169.254.169.254), or 0/8 — POSIX-shell
	// equivalent of validate.isBlockedRange's IPv4 cases, since this runs
	// inside the sandbox container, not the Go binary.
	bootstrap += `
is_public_ipv4() {
  case "$1" in
    10.*|127.*|169.254.*|0.*|192.168.*) return 1 ;;
    172.1[6-9].*|172.2[0-9].*|172.3[0-1].*) return 1 ;;
    *) return 0 ;;
  esac
}
if [ -n "${TARGET_HOST:-}" ]; then
  for ip in $(nslookup "$TARGET_HOST" 2>/dev/null | awk '/^Address: [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/{print $2}'); do
    if is_public_ipv4 "$ip"; then
      iptables -A OUTPUT -d "$ip" -j ACCEPT
    fi
  done
fi`
	bootstrap += " && (command -v ip6tables >/dev/null 2>&1 && ip6tables -P OUTPUT DROP || true)"
	// su-exec nobody resets HOME to "nobody"'s own passwd entry ("/") —
	// confirmed against the real image: `su-exec nobody sh -c 'echo $HOME'`
	// prints "/" even when this whole bootstrap's own process already has
	// HOME=/tmp set (RunSpec.Env, pentestsandbox.Runner). That silently
	// broke ffuf/nuclei/katana (anything that writes a per-user config/cache
	// under $HOME/.config on first run) for every script that goes through
	// this TargetOnly path — they'd fail fast with "open /.config/<tool>/...:
	// no such file or directory" and get recorded as a near-instant
	// exit-code-1 evidence entry, easy to mistake for "ran cleanly, found
	// nothing." `env HOME=/tmp` re-sets it for just the final exec'd
	// command's own environment, after su-exec's reset, rather than
	// depending on an outer Env value su-exec is going to discard anyway.
	bootstrap += ` && exec su-exec nobody env HOME=/tmp "$@"`
	return append([]string{"/bin/sh", "-c", bootstrap, "sh"}, cmd...)
}

// networkMode maps NetworkMode onto Docker's HostConfig.NetworkMode.
// TargetOnly and OpenEgress both get the same permissive "bridge" topology —
// TargetOnly's real restriction to one IP is enforced by firewalledCmd's
// iptables bootstrap above, inside the container's own netns (which holds
// regardless of which Docker network the container is attached to), not by
// this function; OpenEgress deliberately applies no restriction at all, see
// NetworkOpenEgress's own doc comment for why that's the correct choice for
// the one script class it's used for.
func networkMode(mode NetworkMode) container.NetworkMode {
	if mode.Kind == NetworkKindNone {
		return "none"
	}
	return "bridge"
}
