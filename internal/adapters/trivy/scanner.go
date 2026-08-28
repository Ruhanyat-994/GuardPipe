package trivy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// defaultScannerImage is the official Trivy CLI image, pinned to a tag
// rather than a digest — same reasoning as codescan's sonar-scanner-cli:
// this is trusted first-party tooling reading GuardPipe's own cloned
// workspace and a locally-built image, not untrusted input.
const defaultScannerImage = "aquasec/trivy:0.56.2"

// scannerLabelKey/Value mark every container this file creates, mirroring
// adapters/sandbox's own orphan-sweep label and adapters/sonarqube's
// scanner.go so a crashed run doesn't leak a container silently.
const (
	scannerLabelKey   = "guardpipe.containerscan-scanner"
	scannerLabelValue = "true"
)

// dockerSock is the real host Docker socket, bind-mounted into the
// ScanImage container so Trivy can inspect an image BuildImage already
// built locally on this same daemon, without pushing/pulling it anywhere.
// Unlike the workspace directory (backed by a named Docker volume, which is
// why ScanConfig has to mount by volume name + subpath instead — see
// docker-compose.yml's own note on this), the daemon socket is a real host
// device file: bind-mounting it directly is always valid regardless of
// which container initiates the sibling launch.
const dockerSock = "/var/run/docker.sock"

// ScannerConfig is everything Scanner needs beyond the workspace/image
// being analysed — GUARDPIPE_TRIVY_* (documentation/13-devops-and-environments.md
// §5.4).
type ScannerConfig struct {
	Image   string // defaults to defaultScannerImage if empty
	Timeout time.Duration

	// DBUpdate controls whether ScanImage fetches/updates Trivy's
	// vulnerability database over the network on each run
	// (GUARDPIPE_TRIVY_DB_UPDATE). false requires a pre-cached database
	// baked into the image or mounted in some other way — Trivy is passed
	// --skip-db-update --offline-scan in that case rather than failing.
	DBUpdate bool

	// Volume and WorkspaceRoot let ScanConfig mount a workspace directory
	// into the sibling container correctly — the same Docker-outside-of-Docker
	// consideration adapters/sonarqube/scanner.go documents: workspaceDir is
	// a path inside *this* process's own container, not the daemon host's,
	// so it has to be mounted by named-volume + subpath rather than as a
	// plain bind.
	Volume        string
	WorkspaceRoot string

	// CacheVolume, mounted at trivyCacheDir in both ScanConfig and ScanImage,
	// persists Trivy's vulnerability database across runs (a named volume,
	// same shape as Volume above — see docker-compose.yml's volumes.trivy_cache).
	// Without this, every ScanImage call with DBUpdate=true re-downloads the
	// full database (several hundred MB) into a container removed the moment
	// the scan finishes, since nothing ever mounted a place for it to
	// survive to the next run — a real contributor to containerscan blowing
	// GUARDPIPE_ENGINE_TIMEOUT_CONTAINERSCAN, on top of the actual image
	// build/scan work. Empty is still valid (falls back to the image's own
	// ephemeral cache dir) so existing callers/tests that don't set it keep
	// working.
	CacheVolume string
}

// trivyCacheDir is where both trivy commands are told to keep their
// database — an explicit --cache-dir rather than relying on the image's
// default (root's home, which varies by image tag/user), so CacheVolume
// always mounts to the same place trivy actually reads/writes.
const trivyCacheDir = "/trivy-cache"

// Scanner launches short-lived Trivy CLI containers — one for `trivy
// config` (Dockerfile/IaC misconfiguration, no Docker daemon access needed
// by Trivy itself) and one for `trivy image` (vulnerabilities + secrets,
// needs the host socket to see a locally-built image). Deliberately not
// built on adapters/sandbox, for the same reason adapters/sonarqube's
// Scanner isn't: this is trusted first-party tooling that needs network
// access (image pulls, vulnerability-database updates), which
// adapters/sandbox's no-network-by-default policy exists specifically to
// deny.
type Scanner struct {
	docker *dockerx.Client
	cfg    ScannerConfig
}

func NewScanner(docker *dockerx.Client, cfg ScannerConfig) *Scanner {
	if cfg.Image == "" {
		cfg.Image = defaultScannerImage
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}
	return &Scanner{docker: docker, cfg: cfg}
}

// ScanConfig runs `trivy config` against workspaceDir — Dockerfile/IaC
// misconfiguration only, no Docker daemon access required by Trivy itself
// (FR-CNT-001/002/003's "no Docker required" property survives the move to
// Trivy).
func (s *Scanner) ScanConfig(ctx context.Context, workspaceDir string) (Report, error) {
	subpath, err := workspaceSubpath(s.cfg.WorkspaceRoot, workspaceDir)
	if err != nil {
		return Report{}, fmt.Errorf("trivy: %w", err)
	}

	if err := s.docker.PullImage(ctx, s.cfg.Image); err != nil {
		return Report{}, fmt.Errorf("trivy: pull scanner image: %w", err)
	}

	config := &container.Config{
		Image: s.cfg.Image,
		Cmd: append([]string{
			"config", "/workspace", "--format", "json",
			// Restricted to Dockerfile checks only — trivy config's default
			// scope also covers Kubernetes/Terraform/CloudFormation, which is
			// k8sscan's territory (Phase 9), not containerscan's.
			"--misconfig-scanners", "dockerfile",
		}, s.cacheDirArgs()...),
		Labels: map[string]string{scannerLabelKey: scannerLabelValue},
	}
	mounts := []mount.Mount{
		{
			Type:     mount.TypeVolume,
			Source:   s.cfg.Volume,
			Target:   "/workspace",
			ReadOnly: true,
			VolumeOptions: &mount.VolumeOptions{
				Subpath: subpath,
			},
		},
	}
	if m := s.cacheMount(); m != nil {
		mounts = append(mounts, *m)
	}
	hostConfig := &container.HostConfig{Mounts: mounts}

	return s.run(ctx, config, hostConfig, "guardpipe-containerscan-config-")
}

// ScanImage runs `trivy image` against ref — an image tag BuildImage
// already built locally on this daemon (or a registry reference). Needs
// the real host Docker socket bind-mounted so Trivy can see a locally-built
// image without it ever being pushed anywhere.
func (s *Scanner) ScanImage(ctx context.Context, ref string) (Report, error) {
	if err := s.docker.PullImage(ctx, s.cfg.Image); err != nil {
		return Report{}, fmt.Errorf("trivy: pull scanner image: %w", err)
	}

	cmd := []string{"image", ref, "--scanners", "vuln,misconfig,secret", "--format", "json"}
	if !s.cfg.DBUpdate {
		cmd = append(cmd, "--skip-db-update", "--offline-scan")
	}
	cmd = append(cmd, s.cacheDirArgs()...)

	config := &container.Config{
		Image:  s.cfg.Image,
		Cmd:    cmd,
		Labels: map[string]string{scannerLabelKey: scannerLabelValue},
	}
	hostConfig := &container.HostConfig{
		Binds: []string{dockerSock + ":" + dockerSock},
	}
	if m := s.cacheMount(); m != nil {
		hostConfig.Mounts = []mount.Mount{*m}
	}

	return s.run(ctx, config, hostConfig, "guardpipe-containerscan-image-")
}

// cacheDirArgs points trivy at trivyCacheDir explicitly, only when a cache
// volume is actually configured — an empty CacheVolume means "no mount was
// added," so telling trivy to use that path anyway would just fail to
// persist anything instead of falling back cleanly.
func (s *Scanner) cacheDirArgs() []string {
	if s.cfg.CacheVolume == "" {
		return nil
	}
	return []string{"--cache-dir", trivyCacheDir}
}

func (s *Scanner) cacheMount() *mount.Mount {
	if s.cfg.CacheVolume == "" {
		return nil
	}
	return &mount.Mount{Type: mount.TypeVolume, Source: s.cfg.CacheVolume, Target: trivyCacheDir}
}

func (s *Scanner) run(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, namePrefix string) (Report, error) {
	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	name := namePrefix + id.New().String()
	containerID, err := s.docker.CreateContainer(runCtx, config, hostConfig, name)
	if err != nil {
		return Report{}, fmt.Errorf("create scanner container: %w", err)
	}
	defer func() {
		// Background context, same reasoning as adapters/sandbox and
		// adapters/sonarqube: a cancelled/timed-out runCtx must not also
		// cancel cleanup.
		_ = s.docker.RemoveContainer(context.Background(), containerID)
	}()

	if err := s.docker.StartContainer(runCtx, containerID); err != nil {
		return Report{}, fmt.Errorf("start scanner container: %w", err)
	}

	exitCode, waitErr := s.docker.WaitContainer(runCtx, containerID)
	stdout, stderr, logErr := s.docker.Logs(context.Background(), containerID)
	if logErr != nil {
		return Report{}, fmt.Errorf("read scanner logs: %w", logErr)
	}
	if waitErr != nil {
		return Report{}, fmt.Errorf("wait for scanner container: %w", waitErr)
	}
	if exitCode != 0 {
		return Report{}, fmt.Errorf("scanner exited %d: %s", exitCode, lastLines(stderr, stdout))
	}

	report, err := ParseReport(stdout)
	if err != nil {
		return Report{}, fmt.Errorf("parse report: %w", err)
	}
	return report, nil
}

// workspaceSubpath validates that workspaceDir is really inside root and
// returns the relative path Docker's VolumeOptions.Subpath needs.
func workspaceSubpath(root, workspaceDir string) (string, error) {
	subpath, err := filepath.Rel(root, workspaceDir)
	if err != nil || subpath == "." || subpath == ".." || strings.HasPrefix(subpath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspaceDir %q is not inside WorkspaceRoot %q", workspaceDir, root)
	}
	return subpath, nil
}

// lastLines is a small diagnostic helper for error messages — prefers a
// (stderr, then stdout) combination, whichever is non-empty first.
func lastLines(primary, fallback []byte) string {
	if len(primary) > 0 {
		return string(primary)
	}
	return string(fallback)
}
