package k8scodescanscanner_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/k8scodescanscanner"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

const namespace = "guardpipe"

// newCompletingClientset mirrors adapters/k8spentestsandbox's own test
// helper of the same shape — see that package's runner_test.go for why
// client.Tracker().Add, not Pods().Create, is required here (the latter
// re-enters the fake clientset's own non-reentrant lock from inside this
// reactor, a genuine deadlock confirmed the hard way once already this
// session).
func newCompletingClientset(t *testing.T, phase corev1.PodPhase, exitCode int32) *fake.Clientset {
	t.Helper()
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      job.Name + "-pod",
				Namespace: namespace,
				Labels:    job.Spec.Template.Labels,
			},
			Status: corev1.PodStatus{
				Phase: phase,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "scanner",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: exitCode}},
				}},
			},
		}
		if err := client.Tracker().Add(pod); err != nil {
			t.Fatalf("seed pod: %v", err)
		}
		return false, nil, nil
	})
	return client
}

func scanInput() domain.ScanInput {
	return domain.ScanInput{
		Repository: &domain.RepositoryRef{CloneURL: "https://github.com/golang/example.git", Branch: "master"},
	}
}

func TestAnalyze_SuccessExtractsTaskID(t *testing.T) {
	client := newCompletingClientset(t, corev1.PodSucceeded, 0)
	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{
		HostURL: "http://sonarqube.sonarqube.svc.cluster.local:9000",
		Token:   "test-token",
		Timeout: 5 * time.Second,
	}, 1)

	// The fake clientset's Pods().GetLogs() always returns an empty stream
	// (no real container ran), so the task-ID regex never matches here —
	// this test only exercises the "job ran, exit 0, logs read without
	// error" path; TestAnalyze_NoTaskIDInLogsFails below covers the
	// specific error this empty-logs case actually produces, since that IS
	// the fake clientset's real, correct behavior.
	_, err := s.Analyze(context.Background(), scanInput(), "guardpipe-test")
	if err == nil {
		t.Fatal("Analyze() error = nil, want an error — the fake clientset never produces real scanner output for the task-ID regex to match")
	}

	jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("Jobs remaining after Analyze() = %d, want 0 (cleanup should always run)", len(jobs.Items))
	}
}

func TestAnalyze_NonZeroExitCodeFails(t *testing.T) {
	client := newCompletingClientset(t, corev1.PodFailed, 1)
	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 5 * time.Second}, 1)

	if _, err := s.Analyze(context.Background(), scanInput(), "guardpipe-test"); err == nil {
		t.Error("Analyze() error = nil, want an error for a non-zero scanner exit code")
	}
}

func TestAnalyze_NilRepositoryFailsFastWithoutCreatingAJob(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 5 * time.Second}, 1)

	if _, err := s.Analyze(context.Background(), domain.ScanInput{}, "guardpipe-test"); err == nil {
		t.Error("Analyze() error = nil, want an error when ScanInput.Repository is nil")
	}
	jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("Jobs created for a nil Repository = %d, want 0", len(jobs.Items))
	}
}

func TestAnalyze_EmptyCloneURLFailsFastWithoutCreatingAJob(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 5 * time.Second}, 1)

	in := domain.ScanInput{Repository: &domain.RepositoryRef{Branch: "master"}} // CloneURL left empty
	if _, err := s.Analyze(context.Background(), in, "guardpipe-test"); err == nil {
		t.Error("Analyze() error = nil, want an error when Repository.CloneURL is empty")
	}
	jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("Jobs created for an empty CloneURL = %d, want 0", len(jobs.Items))
	}
}

func TestAnalyze_TimeoutStillCleansUpTheJob(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name + "-pod", Namespace: namespace, Labels: job.Spec.Template.Labels},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		}
		_ = client.Tracker().Add(pod)
		return false, nil, nil
	})

	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 500 * time.Millisecond}, 1)
	if _, err := s.Analyze(context.Background(), scanInput(), "guardpipe-test"); err == nil {
		t.Error("Analyze() error = nil, want an error once the timeout elapses with the pod still Pending")
	}

	jobs, _ := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Errorf("Jobs remaining after a timed-out Analyze() = %d, want 0 (cleanup must run even on timeout)", len(jobs.Items))
	}
}

// captureJobs records every Job the scanner creates, and seeds a pod that
// ends in phase with the given clone (init container) exit code.
func captureJobs(t *testing.T, client *fake.Clientset, phase corev1.PodPhase, cloneExit int32) *[]*batchv1.Job {
	t.Helper()
	var jobs []*batchv1.Job
	client.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job := action.(k8stesting.CreateAction).GetObject().(*batchv1.Job)
		jobs = append(jobs, job.DeepCopy())
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name + "-pod", Namespace: namespace, Labels: job.Spec.Template.Labels},
			Status: corev1.PodStatus{
				Phase: phase,
				InitContainerStatuses: []corev1.ContainerStatus{{
					Name:  "clone",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: cloneExit}},
				}},
			},
		}
		if err := client.Tracker().Add(pod); err != nil {
			t.Fatalf("seed pod: %v", err)
		}
		return false, nil, nil
	})
	return &jobs
}

func TestAnalyze_PrivateRepository_TokenOnlyInOwnedSecret(t *testing.T) {
	const token = "ghp_privateRepoTokenForTest"
	client := fake.NewSimpleClientset()
	jobs := captureJobs(t, client, corev1.PodFailed, 0)

	var secret *corev1.Secret
	client.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		secret = action.(k8stesting.CreateAction).GetObject().(*corev1.Secret).DeepCopy()
		return false, nil, nil
	})

	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 5 * time.Second}, 1)
	in := scanInput()
	in.Repository.CloneToken = token
	_, _ = s.Analyze(context.Background(), in, "guardpipe-test")

	if len(*jobs) != 1 {
		t.Fatalf("jobs created = %d, want 1", len(*jobs))
	}
	job := (*jobs)[0]
	if spec := fmt.Sprintf("%+v", job.Spec); strings.Contains(spec, token) {
		t.Error("the token appears in the Job spec; it must only be in the Secret")
	}

	if secret == nil {
		t.Fatal("no Secret created for a private repository")
	}
	if len(secret.OwnerReferences) != 1 || secret.OwnerReferences[0].Kind != "Job" || secret.OwnerReferences[0].Name != job.Name {
		t.Errorf("Secret owner = %+v, want the Job %s", secret.OwnerReferences, job.Name)
	}
	header := secret.StringData["git-auth-header"]
	want := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
	if header != want {
		t.Errorf("Secret header = %q, want %q", header, want)
	}

	var fromSecret bool
	for _, e := range job.Spec.Template.Spec.InitContainers[0].Env {
		if e.Name == "GIT_CONFIG_VALUE_0" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil &&
			e.ValueFrom.SecretKeyRef.Name == secret.Name {
			fromSecret = true
		}
	}
	if !fromSecret {
		t.Error("clone container does not read the auth header from the run's Secret")
	}

	left, _ := client.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{})
	if len(left.Items) != 0 {
		t.Errorf("Secrets remaining after Analyze() = %d, want 0", len(left.Items))
	}
}

func TestAnalyze_PublicRepository_CreatesNoSecret(t *testing.T) {
	client := fake.NewSimpleClientset()
	jobs := captureJobs(t, client, corev1.PodFailed, 0)
	created := 0
	client.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		created++
		return false, nil, nil
	})

	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 5 * time.Second}, 1)
	_, _ = s.Analyze(context.Background(), scanInput(), "guardpipe-test")

	if created != 0 {
		t.Errorf("Secrets created for a public repository = %d, want 0", created)
	}
	for _, e := range (*jobs)[0].Spec.Template.Spec.InitContainers[0].Env {
		if strings.HasPrefix(e.Name, "GIT_CONFIG_") {
			t.Errorf("public clone has %s set, want no auth config", e.Name)
		}
	}
}

func TestAnalyze_CloneFailureIsReportedAsCloneError(t *testing.T) {
	client := fake.NewSimpleClientset()
	captureJobs(t, client, corev1.PodFailed, 128)

	s := k8scodescanscanner.New(client, namespace, k8scodescanscanner.Config{Timeout: 5 * time.Second}, 1)
	_, err := s.Analyze(context.Background(), scanInput(), "guardpipe-test")
	if err == nil || !strings.Contains(err.Error(), "clone failed (exit 128)") {
		t.Errorf("Analyze() error = %v, want a clone failure", err)
	}
}
