package sonarqube

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// defaultScannerImage is the official SonarSource scanner CLI image.
// Pinned to a tag rather than a digest for now — adapters/sandbox's RunSpec
// documents digest-pinning as the standard for anything running untrusted
// input, which this isn't (sonar-scanner is a trusted first-party tool
// reading GuardPipe's own cloned workspace, the same trust level as the
// `git clone` step itself). Tightening this to a digest is a straightforward
// follow-up, not a correctness gap.
//
// 12.1, not 5.0: 5.0 bundles a Java 17 JRE, which self-hosted SonarQube
// Community Build (the sonarqube:community image pulled here, version
// 26.8.0 as of writing) refuses to analyse against — "Java 17 is not
// supported. Please upgrade to Java 21 or newer." 12.1 bundles Java 21
// (Corretto 21.0.11), verified against this project's actual sonarqube
// service. If a future sonarqube:community bump raises the floor again,
// this is the first place to check.
const defaultScannerImage = "sonarsource/sonar-scanner-cli:12.1"

// scannerLabelKey/Value mark every container this file creates, mirroring
// adapters/sandbox's own orphan-sweep label so a crashed run doesn't leak a
// container silently.
const (
	scannerLabelKey   = "guardpipe.codescan-scanner"
	scannerLabelValue = "true"
)

// taskIDPattern extracts the SonarQube background-task ID from
// sonar-scanner's own stdout on a successful submission — the line reads
// "More about the report processing at <host>/api/ce/task?id=<TASK_ID>".
// This is the standard, stable way every sonar-scanner CLI reports which
// task to poll; it doesn't require reading files back out of a removed
// container.
var taskIDPattern = regexp.MustCompile(`ce/task\?id=([A-Za-z0-9_-]+)`)

// ScannerConfig is everything Scanner needs beyond the workspace/project
// being analysed — GUARDPIPE_SONARQUBE_* plus GUARDPIPE_DOCKER_NETWORK
// (documentation/13-devops-and-environments.md §5.4/§5.7).
type ScannerConfig struct {
	Image   string // defaults to defaultScannerImage if empty
	Network string // Compose network name the scanner container joins to reach the sonarqube service by hostname
	HostURL string // SonarQube URL reachable from that network, e.g. "http://sonarqube:9000" — not necessarily GUARDPIPE_SONARQUBE_API_URL if that's an external-facing URL
	Token   string
	Timeout time.Duration

	// Volume and WorkspaceRoot together let Analyze mount a workspace
	// directory into the sibling scanner container correctly. workspaceDir
	// (the argument to Analyze) is a path inside *this* process's own
	// container — e.g. /var/lib/guardpipe/workspace/scan-abc123 — but the
	// scanner container is created via the shared Docker socket, so a plain
	// bind-mount of that path is resolved against the daemon host's
	// filesystem, not this container's, and silently mounts an empty
	// directory instead. Mounting Volume (GUARDPIPE_WORKSPACE_VOLUME, the
	// same named volume WorkspaceRoot is backed by) with a Subpath for
	// workspaceDir's own subdirectory is what actually reaches the cloned
	// checkout.
	Volume        string
	WorkspaceRoot string
}

// Scanner launches a short-lived sonar-scanner-cli container against an
// already-cloned workspace. It is deliberately not built on
// adapters/sandbox: that package enforces a no-network-by-default policy for
// *untrusted* execution (pentest scripts, container image extraction),
// which doesn't apply here — sonar-scanner is trusted, first-party tooling
// that must reach the sonarqube service over the network to do its job at
// all.
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

// Analyze runs sonar-scanner-cli against workspaceDir under projectKey and
// returns the SonarQube background task ID it submitted to. The caller
// (engines/codescan) polls Client.GetTask with that ID until the analysis
// finishes. A non-zero scanner exit code, or a run that never prints a task
// ID, is returned as an error — codescan's job fails, the rest of the scan
// continues (documentation/05-module-specifications.md §6's Failure modes
// table).
func (s *Scanner) Analyze(ctx context.Context, workspaceDir, projectKey string) (taskID string, err error) {
	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	subpath, err := filepath.Rel(s.cfg.WorkspaceRoot, workspaceDir)
	if err != nil || subpath == "." || subpath == ".." || strings.HasPrefix(subpath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sonarqube: workspaceDir %q is not inside WorkspaceRoot %q", workspaceDir, s.cfg.WorkspaceRoot)
	}

	if err := s.docker.PullImage(runCtx, s.cfg.Image); err != nil {
		return "", fmt.Errorf("sonarqube: pull scanner image: %w", err)
	}

	config := &container.Config{
		Image: s.cfg.Image,
		Env: []string{
			"SONAR_HOST_URL=" + s.cfg.HostURL,
			"SONAR_TOKEN=" + s.cfg.Token,
		},
		Cmd: []string{
			"-Dsonar.projectKey=" + projectKey,
			// sonar-scanner-cli's own image mounts the source at /usr/src by
			// convention; that mount is read-only (workspace is read-only to
			// engines — domain.ScanInput's doc comment), so the scanner's own
			// working directory has to live somewhere writable instead.
			"-Dsonar.working.directory=/tmp/scannerwork",
			"-Dsonar.sources=/usr/src",
			// Shallow clones (git clone --depth 1, adapters/github) have no
			// blame history for SonarQube's SCM-based features to read —
			// disabling SCM avoids the scanner failing on that, not a
			// meaningful loss (GuardPipe doesn't use SonarQube's SCM-derived
			// data anyway).
			"-Dsonar.scm.disabled=true",
			// Without this, sonar-scanner walks and indexes -Dsonar.sources's
			// entire tree, including dependency/build output directories that
			// were never meant to be analysed — engines/codescan's own
			// Applicable() and engines/containerscan's findDockerfile() both
			// already skip exactly this set (their own skipDirs) when deciding
			// whether to run at all, but that pre-check never told the scanner
			// container itself to skip them too. Confirmed against this
			// project's own checkout: frontend/node_modules alone is
			// 236MB/17k+ files, several times the size of every real source
			// file combined — indexing it (plus re-parsing thousands of
			// third-party .js/.ts files SonarQube's own analyzers pick up as
			// sources) is what pushed real analyses past
			// GUARDPIPE_ENGINE_TIMEOUT_CODESCAN, not a genuinely slow analysis
			// of GuardPipe's own code.
			"-Dsonar.exclusions=**/node_modules/**,**/.git/**,**/vendor/**,**/dist/**,**/build/**,**/.venv/**,**/__pycache__/**",
		},
		Labels: map[string]string{scannerLabelKey: scannerLabelValue},
	}
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(s.cfg.Network),
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeVolume,
				Source:   s.cfg.Volume,
				Target:   "/usr/src",
				ReadOnly: true,
				VolumeOptions: &mount.VolumeOptions{
					Subpath: subpath,
				},
			},
		},
		Tmpfs: map[string]string{"/tmp": ""},
	}

	name := "guardpipe-codescan-" + id.New().String()
	containerID, err := s.docker.CreateContainer(runCtx, config, hostConfig, name)
	if err != nil {
		return "", fmt.Errorf("sonarqube: create scanner container: %w", err)
	}
	defer func() {
		// Background context, same reasoning as adapters/sandbox: a
		// cancelled/timed-out runCtx must not also cancel cleanup.
		_ = s.docker.RemoveContainer(context.Background(), containerID)
	}()

	if err := s.docker.StartContainer(runCtx, containerID); err != nil {
		return "", fmt.Errorf("sonarqube: start scanner container: %w", err)
	}

	exitCode, waitErr := s.docker.WaitContainer(runCtx, containerID)
	stdout, stderr, logErr := s.docker.Logs(context.Background(), containerID)
	if logErr != nil {
		return "", fmt.Errorf("sonarqube: read scanner logs: %w", logErr)
	}
	if waitErr != nil {
		return "", fmt.Errorf("sonarqube: wait for scanner container: %w", waitErr)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("sonarqube: scanner exited %d: %s", exitCode, lastLines(stderr, stdout))
	}

	match := taskIDPattern.FindSubmatch(stdout)
	if match == nil {
		return "", fmt.Errorf("sonarqube: scanner exited 0 but reported no background task ID: %s", lastLines(stdout, stderr))
	}
	return string(match[1]), nil
}

// lastLines is a small diagnostic helper for error messages — prefers a
// (stderr, then stdout) combination, whichever is non-empty first, so a
// scanner failure's error message actually says something useful instead of
// "exit 1" with no context.
func lastLines(primary, fallback []byte) string {
	if len(primary) > 0 {
		return string(primary)
	}
	return string(fallback)
}
