// Package k8strivyscanner implements containerscan.Scanner (internal/engines/
// containerscan/engine.go) by running Trivy as a one-shot Kubernetes Job,
// instead of adapters/trivy.Scanner's Docker sibling container. Built for the
// EKS deployment, which has no Docker socket reachable from guardpipe-worker
// at all (the same reason adapters/k8scodescanscanner and
// adapters/k8spentestsandbox exist); picked by the same
// GUARDPIPE_SANDBOX_BACKEND=kubernetes switch in cmd/guardpipe/main.go.
//
// Two differences from the Docker path, both forced by "no Docker":
//
//   - ScanConfig doesn't mount the worker's checkout (no shared volume exists
//     in this cluster). Only the Dockerfile(s) are needed for a
//     `--misconfig-scanners dockerfile` scan, and they're small, so they're
//     shipped to the Job in a per-run ConfigMap — no clone, no credentials.
//   - Without Docker there is no image build, so the engine hands ScanImage a
//     public image reference (the Dockerfile's base image) rather than a
//     locally built tag; Trivy pulls it straight from its registry.
//
// The image reference comes from a scanned repository — hostile input by
// design — so it is validated and passed to the container as a positional
// argument, never interpolated into the shell script.
package k8strivyscanner

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// defaultImage matches adapters/trivy's own defaultScannerImage — the same
// Trivy version on both backends, so the two produce comparable reports.
const defaultImage = "aquasec/trivy:0.56.2"

// RunLabelKey marks every Job and ConfigMap this package creates; the
// allow-containerscan-job-egress NetworkPolicy (deploy/k8s/08-networkpolicy.yaml)
// selects on it with `Exists`.
const RunLabelKey = "guardpipe.io/containerscan-run"

const (
	runAsNobody    int64 = 65534
	defaultTimeout       = 10 * time.Minute
	pollInterval         = 3 * time.Second
	memoryLimitMB        = 1536
	// maxDockerfileBytes caps what goes into one ConfigMap (the API's own
	// hard limit is 1 MiB); real Dockerfiles are a few KB.
	maxDockerfileBytes = 512 * 1024
	reportMarker       = "===GUARDPIPE-TRIVY-REPORT==="
)

// imageRefPattern is a conservative image reference: registry/path[:tag][@digest].
// Anything else (spaces, shell metacharacters, variables) is refused.
var imageRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-/:@]{0,254}$`)

type Config struct {
	Image   string // defaults to defaultImage
	Timeout time.Duration
}

type Scanner struct {
	client    kubernetes.Interface
	namespace string
	cfg       Config
	sem       chan struct{}
}

// New constructs a Scanner. maxConcurrent bounds concurrent Trivy Jobs
// against the cluster's small node capacity (same reasoning as
// k8scodescanscanner.New); <= 0 is treated as 1.
func New(client kubernetes.Interface, namespace string, cfg Config, maxConcurrent int) *Scanner {
	if cfg.Image == "" {
		cfg.Image = defaultImage
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Scanner{client: client, namespace: namespace, cfg: cfg, sem: make(chan struct{}, maxConcurrent)}
}

// ScanConfig runs `trivy config` (Dockerfile checks only, like the Docker
// path) over every Dockerfile in workspaceDir. No Dockerfile → empty report.
func (s *Scanner) ScanConfig(ctx context.Context, workspaceDir string) (trivy.Report, error) {
	files, err := collectDockerfiles(workspaceDir)
	if err != nil {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: collect dockerfiles: %w", err)
	}
	if len(files) == 0 {
		return trivy.Report{}, nil
	}
	script := `trivy config /work --format json --misconfig-scanners dockerfile --quiet ` +
		`--cache-dir /tmp/trivy-cache --output /tmp/report.json && echo ` + reportMarker + ` && cat /tmp/report.json`
	return s.run(ctx, "config", []string{script}, files)
}

// ScanImage runs `trivy image` against ref, pulled by Trivy itself from its
// registry (no Docker daemon involved).
func (s *Scanner) ScanImage(ctx context.Context, ref string) (trivy.Report, error) {
	if !imageRefPattern.MatchString(ref) || strings.Contains(ref, "..") {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: refusing to scan unrecognised image reference %q", ref)
	}
	// "$1" is the image reference, bound by `sh -c script sh ref` — never
	// part of the script text itself.
	script := `trivy image "$1" --scanners vuln,secret --format json --quiet ` +
		`--cache-dir /tmp/trivy-cache --output /tmp/report.json && echo ` + reportMarker + ` && cat /tmp/report.json`
	return s.run(ctx, "image", []string{script, "sh", ref}, nil)
}

func (s *Scanner) run(ctx context.Context, kind string, shArgs []string, files map[string]string) (trivy.Report, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return trivy.Report{}, ctx.Err()
	}

	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	runID := "containerscan-" + kind + "-" + id.New().String()
	labels := map[string]string{RunLabelKey: runID}

	// The Job is created first so the ConfigMap can name it as its owner:
	// Kubernetes then garbage-collects the ConfigMap with the Job (TTL or
	// delete), even if this worker dies mid-scan. Until the ConfigMap
	// exists the pod simply waits for its volume.
	var items []corev1.KeyToPath
	i := 0
	for path := range files {
		items = append(items, corev1.KeyToPath{Key: fmt.Sprintf("f%d", i), Path: path})
		i++
	}
	job, err := s.client.BatchV1().Jobs(s.namespace).Create(runCtx, s.job(runID, labels, shArgs, items), metav1.CreateOptions{})
	if err != nil {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: create job: %w", err)
	}
	defer s.deleteJob(runID)

	if len(items) > 0 {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: runID, Namespace: s.namespace, Labels: labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "batch/v1", Kind: "Job", Name: job.Name, UID: job.UID,
				}},
			},
			Data: map[string]string{},
		}
		for _, it := range items {
			cm.Data[it.Key] = files[it.Path]
		}
		if _, err := s.client.CoreV1().ConfigMaps(s.namespace).Create(runCtx, cm, metav1.CreateOptions{}); err != nil {
			return trivy.Report{}, fmt.Errorf("k8strivyscanner: create configmap: %w", err)
		}
		defer func() {
			_ = s.client.CoreV1().ConfigMaps(s.namespace).Delete(context.Background(), runID, metav1.DeleteOptions{})
		}()
	}

	pod, waitErr := s.waitForPodCompletion(runCtx, runID)
	if waitErr != nil {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: wait for job: %w", waitErr)
	}
	if pod == nil {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: job %s never scheduled a pod before the deadline", runID)
	}

	logs, err := s.podLogs(context.Background(), pod.Name)
	if err != nil {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: read trivy logs: %w", err)
	}
	exitCode := int32(-1)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "trivy" && cs.State.Terminated != nil {
			exitCode = cs.State.Terminated.ExitCode
		}
	}
	if exitCode != 0 {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: trivy %s exited %d: %s", kind, exitCode, lastLines(logs))
	}
	return extractReport(logs)
}

func (s *Scanner) job(runID string, labels map[string]string, shArgs []string, items []corev1.KeyToPath) *batchv1.Job {
	trueVal, falseVal := true, false
	uid := runAsNobody
	activeDeadline := int64(s.cfg.Timeout.Seconds())
	backoffLimit := int32(0)
	ttl := int32(300)

	volumes := []corev1.Volume{{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	mounts := []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}}
	if len(items) > 0 {
		volumes = append(volumes, corev1.Volume{Name: "work", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: runID}, Items: items},
		}})
		mounts = append(mounts, corev1.VolumeMount{Name: "work", MountPath: "/work", ReadOnly: true})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: runID, Namespace: s.namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &activeDeadline,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: &falseVal,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &trueVal,
						RunAsUser:      &uid,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:  "trivy",
						Image: s.cfg.Image,
						// Command, not Args: this replaces the image's trivy
						// ENTRYPOINT with sh so the report can be marked off
						// from Trivy's own log lines on the same stream.
						Command: append([]string{"sh", "-c"}, shArgs...),
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &trueVal,
							RunAsUser:                &uid,
							ReadOnlyRootFilesystem:   &trueVal,
							AllowPrivilegeEscalation: &falseVal,
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						},
						// UID 65534 has no passwd entry, so HOME would be "/",
						// read-only here; Trivy's temp files need a writable dir.
						Env:          []corev1.EnvVar{{Name: "HOME", Value: "/tmp"}, {Name: "TMPDIR", Value: "/tmp"}},
						VolumeMounts: mounts,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memoryLimitMB)),
								corev1.ResourceCPU:    resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
								corev1.ResourceCPU:    resource.MustParse("250m"),
							},
						},
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func (s *Scanner) waitForPodCompletion(ctx context.Context, runID string) (*corev1.Pod, error) {
	var latest *corev1.Pod
	err := wait.PollUntilContextCancel(ctx, pollInterval, true, func(ctx context.Context) (bool, error) {
		pods, err := s.client.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{LabelSelector: RunLabelKey + "=" + runID})
		if err != nil {
			return false, err
		}
		if len(pods.Items) == 0 {
			return false, nil
		}
		pod := pods.Items[0]
		latest = &pod
		return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed, nil
	})
	return latest, err
}

func (s *Scanner) podLogs(ctx context.Context, podName string) ([]byte, error) {
	stream, err := s.client.CoreV1().Pods(s.namespace).GetLogs(podName, &corev1.PodLogOptions{Container: "trivy"}).Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(stream); err != nil {
		return buf.Bytes(), nil //nolint:nilerr // partial logs still carry the error text the caller reports
	}
	return buf.Bytes(), nil
}

func (s *Scanner) deleteJob(name string) {
	fg := metav1.DeletePropagationForeground
	_ = s.client.BatchV1().Jobs(s.namespace).Delete(context.Background(), name, metav1.DeleteOptions{PropagationPolicy: &fg})
}

// SweepOrphans removes every Job this package's label marks — run once at
// startup so a worker crash mid-scan never leaves one behind. Their
// ConfigMaps go with them (owner references), so they need no sweep.
func (s *Scanner) SweepOrphans(ctx context.Context) (int, error) {
	opts := metav1.ListOptions{LabelSelector: RunLabelKey}
	jobs, err := s.client.BatchV1().Jobs(s.namespace).List(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("k8strivyscanner: list orphaned jobs: %w", err)
	}
	fg := metav1.DeletePropagationForeground
	removed := 0
	for _, j := range jobs.Items {
		if err := s.client.BatchV1().Jobs(s.namespace).Delete(ctx, j.Name, metav1.DeleteOptions{PropagationPolicy: &fg}); err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("k8strivyscanner: remove orphaned job %s: %w", j.Name, err)
		}
		removed++
	}
	return removed, nil
}

// extractReport parses the JSON printed after reportMarker; anything before
// it is Trivy's own log output.
func extractReport(logs []byte) (trivy.Report, error) {
	i := bytes.LastIndex(logs, []byte(reportMarker))
	if i < 0 {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: trivy exited 0 but printed no report: %s", lastLines(logs))
	}
	report, err := trivy.ParseReport(bytes.TrimSpace(logs[i+len(reportMarker):]))
	if err != nil {
		return trivy.Report{}, fmt.Errorf("k8strivyscanner: parse report: %w", err)
	}
	return report, nil
}

// skipDirs mirrors engines/containerscan's own Dockerfile discovery.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, ".venv": true, "__pycache__": true,
}

// isDockerfileName mirrors engines/containerscan.isDockerfileName.
func isDockerfileName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "dockerfile" || lower == "containerfile" || strings.HasSuffix(lower, ".dockerfile")
}

// collectDockerfiles returns every Dockerfile under root, keyed by its
// slash-separated path relative to root.
func collectDockerfiles(root string) (map[string]string, error) {
	out := map[string]string{}
	total := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree just isn't scanned
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !isDockerfileName(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // same: skip what can't be read
		}
		if total+len(data) > maxDockerfileBytes {
			return fmt.Errorf("dockerfiles exceed %d bytes", maxDockerfileBytes)
		}
		total += len(data)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out, err
}

func lastLines(b []byte) string {
	const maxLen = 2000
	if len(b) > maxLen {
		b = b[len(b)-maxLen:]
	}
	return string(b)
}
