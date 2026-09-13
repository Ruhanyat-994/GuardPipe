// Package k8scodescanscanner implements codescan.Scanner (internal/engines/
// codescan/engine.go) by running sonar-scanner-cli as a one-shot Kubernetes
// Job, instead of adapters/sonarqube.Scanner's Docker sidecar container.
// Built for the EKS deployment, which has no Docker socket reachable from
// guardpipe-worker at all — see cmd/guardpipe/main.go's
// GUARDPIPE_SANDBOX_BACKEND wiring, shared with adapters/k8spentestsandbox
// (both engines that need it always agree on which backend a deployment
// uses). The Docker Compose path (adapters/sonarqube) is untouched; this is
// a sibling implementation of the same engine-facing interface, not a
// replacement.
//
// Unlike adapters/k8spentestsandbox, this package's Job pods don't share a
// filesystem with guardpipe-worker's own already-cloned checkout (no
// ReadWriteMany volume exists in this cluster, and a git checkout is too
// large for a ConfigMap the way pentest's small dynamic wordlists are) — so
// each Job's own init container clones the repository itself, fresh,
// shallow (--depth 1), the same strategy adapters/github already uses for
// the original clone.
//
// Public repositories only, for now: no decrypted credential reaches an
// engine today (domain.ScanInput carries no PAT), and plumbing one through
// from modules/project's credential store would be a materially bigger,
// separate change — a real, accepted limitation, not a silent gap. A
// private repository's RepositoryRef.CloneURL is populated the same way a
// public one's is (orchestrator.Pool's own worker.go, straight from
// modules/project.Service.GetCloneInfo — the same URL the orchestrator's
// own clone into ScanInput.WorkspaceDir already used); the Job's clone
// simply fails with a normal git-auth error, surfaced as this engine's own
// job error like any other codescan failure.
package k8scodescanscanner

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// scannerImage/cloneImage are pinned the same way every other tool image in
// this project is — scannerImage matches adapters/sonarqube/scanner.go's
// own defaultScannerImage exactly (same version, same reasoning: 12.1
// bundles a Java 21 JRE the self-hosted sonarqube:community image actually
// needs — see that file's own doc comment for the full story). Kept as a
// separate constant, not exported/shared, so this package's tests don't
// depend on the Docker adapter package.
const (
	scannerImage = "sonarsource/sonar-scanner-cli:12.1"
	cloneImage   = "alpine/git:2.43.0"
)

const runLabelKey = "guardpipe.io/codescan-run"
const runAsNobody int64 = 65534

const (
	defaultMemoryMB = 1024
	defaultCPUs     = 1.0
	pollInterval    = 3 * time.Second
	defaultTimeout  = 10 * time.Minute
)

var taskIDPattern = regexp.MustCompile(`ce/task\?id=([A-Za-z0-9_-]+)`)

// Config is everything Scanner needs beyond the ScanInput/projectKey Analyze
// receives per call.
type Config struct {
	HostURL string // GUARDPIPE_SONARQUBE_API_URL, reachable from inside the cluster
	Token   string
	Timeout time.Duration
}

type Scanner struct {
	client    kubernetes.Interface
	namespace string
	cfg       Config
	sem       chan struct{}
}

// New constructs a Scanner. maxConcurrent mirrors adapters/k8spentestsandbox.New's
// own reasoning — bounds concurrent Job pods against this cluster's real,
// small node capacity; a value <= 0 is treated as 1, never unbounded.
func New(client kubernetes.Interface, namespace string, cfg Config, maxConcurrent int) *Scanner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Scanner{client: client, namespace: namespace, cfg: cfg, sem: make(chan struct{}, maxConcurrent)}
}

// Analyze runs sonar-scanner-cli against a fresh clone of in.Repository and
// returns the SonarQube background task ID it submitted to — see the
// package doc comment for why this clones itself rather than reading
// in.WorkspaceDir (the Docker path's own approach), and for the public-
// repository-only limitation of this first version.
func (s *Scanner) Analyze(ctx context.Context, in domain.ScanInput, projectKey string) (taskID string, err error) {
	if in.Repository == nil {
		return "", fmt.Errorf("k8scodescanscanner: ScanInput.Repository is nil — nothing to clone")
	}

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	runID := "codescan-" + id.New().String()
	labels := map[string]string{runLabelKey: runID}

	if in.Repository.CloneURL == "" {
		return "", fmt.Errorf("k8scodescanscanner: ScanInput.Repository.CloneURL is empty — nothing to clone")
	}
	ref := in.Repository.CommitSHA
	if ref == "" {
		ref = in.Repository.Branch
	}

	jobName, err := s.createJob(runCtx, runID, labels, in.Repository.CloneURL, ref, projectKey)
	if err != nil {
		return "", fmt.Errorf("k8scodescanscanner: create job: %w", err)
	}
	defer s.deleteJob(jobName)

	pod, waitErr := s.waitForPodCompletion(runCtx, runID)
	if waitErr != nil {
		return "", fmt.Errorf("k8scodescanscanner: wait for job: %w", waitErr)
	}
	if pod == nil {
		return "", fmt.Errorf("k8scodescanscanner: job %s never scheduled a pod before the deadline", jobName)
	}

	logs, err := s.podLogs(context.Background(), pod.Name, "scanner")
	if err != nil {
		return "", fmt.Errorf("k8scodescanscanner: read scanner container logs: %w", err)
	}

	exitCode := int32(-1)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "scanner" && cs.State.Terminated != nil {
			exitCode = cs.State.Terminated.ExitCode
		}
	}
	if exitCode != 0 {
		return "", fmt.Errorf("k8scodescanscanner: scanner exited %d: %s", exitCode, lastLines(logs))
	}

	match := taskIDPattern.FindSubmatch(logs)
	if match == nil {
		return "", fmt.Errorf("k8scodescanscanner: scanner exited 0 but reported no background task ID: %s", lastLines(logs))
	}
	return string(match[1]), nil
}

func (s *Scanner) createJob(ctx context.Context, runID string, labels map[string]string, cloneURL, ref, projectKey string) (string, error) {
	trueVal, falseVal := true, false
	uid := runAsNobody
	activeDeadline := int64(s.cfg.Timeout.Seconds())
	backoffLimit := int32(0)

	securityContext := &corev1.SecurityContext{
		RunAsNonRoot:             &trueVal,
		RunAsUser:                &uid,
		ReadOnlyRootFilesystem:   &trueVal,
		AllowPrivilegeEscalation: &falseVal,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: runID, Namespace: s.namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &activeDeadline,
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &trueVal,
						RunAsUser:      &uid,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					InitContainers: []corev1.Container{{
						Name:            "clone",
						Image:           cloneImage,
						SecurityContext: securityContext,
						// HOME=/tmp: the numeric UID this runs as (65534,
						// "nobody") has no real passwd entry in this image,
						// so git's default HOME resolves to "/" — not
						// writable under ReadOnlyRootFilesystem, and git
						// itself (global config lookup, credential helper
						// probing) touches HOME even for a plain clone.
						// Same class of bug already hit and fixed once this
						// session for the pentest sandbox's own tool
						// containers (adapters/pentestsandbox.Runner.Run's
						// own HOME entry) — applied here proactively rather
						// than waiting to rediscover it live.
						Env: []corev1.EnvVar{{Name: "HOME", Value: "/tmp"}},
						Command: []string{
							"git", "clone", "--depth", "1", "--branch", ref, cloneURL, "/workspace/src",
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "workspace", MountPath: "/workspace"},
							{Name: "tmp", MountPath: "/tmp"},
						},
					}},
					Containers: []corev1.Container{{
						Name:            "scanner",
						Image:           scannerImage,
						SecurityContext: securityContext,
						Env: []corev1.EnvVar{
							{Name: "SONAR_HOST_URL", Value: s.cfg.HostURL},
							{Name: "SONAR_TOKEN", Value: s.cfg.Token},
						},
						Command: []string{
							"-Dsonar.projectKey=" + projectKey,
							"-Dsonar.working.directory=/tmp/scannerwork",
							"-Dsonar.sources=/workspace/src",
							// Shallow clone (--depth 1 above) has no blame
							// history for SonarQube's SCM-based features to
							// read — same reasoning adapters/sonarqube/
							// scanner.go's own comment gives.
							"-Dsonar.scm.disabled=true",
							// Same exclusion list adapters/sonarqube/
							// scanner.go already uses — see its own doc
							// comment for why (dependency/build-output
							// directories were never meant to be analysed,
							// and indexing them is what pushed real
							// analyses past the engine timeout, not slow
							// analysis of real source).
							"-Dsonar.exclusions=**/node_modules/**,**/.git/**,**/vendor/**,**/dist/**,**/build/**,**/.venv/**,**/__pycache__/**",
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "workspace", MountPath: "/workspace"},
							{Name: "tmp", MountPath: "/tmp"},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", defaultMemoryMB)),
								corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%g", defaultCPUs)),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", defaultMemoryMB/2)),
								corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%g", defaultCPUs/4)),
							},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
				},
			},
		},
	}

	created, err := s.client.BatchV1().Jobs(s.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return created.Name, nil
}

func (s *Scanner) waitForPodCompletion(ctx context.Context, runID string) (*corev1.Pod, error) {
	var latest *corev1.Pod
	err := wait.PollUntilContextCancel(ctx, pollInterval, true, func(ctx context.Context) (bool, error) {
		pods, err := s.client.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: runLabelKey + "=" + runID,
		})
		if err != nil {
			return false, err
		}
		if len(pods.Items) == 0 {
			return false, nil
		}
		pod := pods.Items[0]
		latest = &pod
		switch pod.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			return true, nil
		default:
			return false, nil
		}
	})
	return latest, err
}

func (s *Scanner) podLogs(ctx context.Context, podName, container string) ([]byte, error) {
	req := s.client.CoreV1().Pods(s.namespace).GetLogs(podName, &corev1.PodLogOptions{Container: container})
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(stream); err != nil {
		return buf.Bytes(), nil //nolint:nilerr // partial logs are still useful; the caller only needs bytes to search
	}
	return buf.Bytes(), nil
}

func (s *Scanner) deleteJob(name string) {
	fg := metav1.DeletePropagationForeground
	_ = s.client.BatchV1().Jobs(s.namespace).Delete(context.Background(), name, metav1.DeleteOptions{PropagationPolicy: &fg})
}

// SweepOrphans force-removes every Job this package's label marks, run once
// at startup — mirrors adapters/k8spentestsandbox.Runner.SweepOrphans and
// adapters/sandbox.DockerSandbox.SweepOrphans exactly: a Job leaked by a
// crash mid-run should never linger. Returns the number removed.
func (s *Scanner) SweepOrphans(ctx context.Context) (int, error) {
	jobs, err := s.client.BatchV1().Jobs(s.namespace).List(ctx, metav1.ListOptions{LabelSelector: runLabelKey})
	if err != nil {
		return 0, fmt.Errorf("k8scodescanscanner: list orphaned jobs: %w", err)
	}
	fg := metav1.DeletePropagationForeground
	removed := 0
	for _, j := range jobs.Items {
		if err := s.client.BatchV1().Jobs(s.namespace).Delete(ctx, j.Name, metav1.DeleteOptions{PropagationPolicy: &fg}); err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("k8scodescanscanner: remove orphaned job %s: %w", j.Name, err)
		}
		removed++
	}
	return removed, nil
}

func lastLines(b []byte) string {
	const maxLen = 2000
	if len(b) > maxLen {
		b = b[len(b)-maxLen:]
	}
	return string(b)
}

func ptrInt32(v int32) *int32 { return &v }
