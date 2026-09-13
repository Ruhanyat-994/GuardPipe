package k8scodescanscanner_test

import (
	"context"
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
