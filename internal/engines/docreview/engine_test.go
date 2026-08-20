package docreview

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func TestEngine_Applicable_WithUploadedDocument(t *testing.T) {
	e := New(nil)
	in := domain.ScanInput{
		WorkspaceDir: t.TempDir(),
		Documents:    []domain.DocumentRef{{Path: "srs.md", Content: "# SRS"}},
	}
	ok, reason := e.Applicable(context.Background(), in)
	if !ok {
		t.Errorf("Applicable() = false (%q), want true — an uploaded document alone must be enough", reason)
	}
}

func TestEngine_Applicable_WithRepoDiscoveredDocument(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/architecture.md", "# Architecture")

	e := New(nil)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if !ok {
		t.Errorf("Applicable() = false (%q), want true", reason)
	}
}

// TestEngine_Applicable_NoDocuments is the near-miss: neither an uploaded
// document nor a matching repository file must not falsely report itself
// applicable (documentation/05-module-specifications.md §11's own
// failure-mode table: "No documents found -> Applicable false -> skipped").
func TestEngine_Applicable_NoDocuments(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "package main") // present, but not a reviewable document

	e := New(nil)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	if ok {
		t.Error("Applicable() = true, want false — no documents exist anywhere")
	}
	if reason == "" {
		t.Error("Applicable() reason is empty, want an explanation")
	}
}

// TestEngine_Run_NilAIService confirms docreview's documented "no
// deterministic fallback" behaviour (documentation/05-module-specifications.md
// §11: "Gemini unavailable -> Job failed with ai_unavailable") — unlike
// cicdscan's graceful degrade, a nil aiSvc must fail the job outright, never
// silently succeed with zero findings (which would look identical to a
// genuinely clean document set).
func TestEngine_Run_NilAIService(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/architecture.md", "# Architecture")

	e := New(nil)
	ctx := context.Background()
	in := domain.ScanInput{ScanID: uuid.New(), WorkspaceDir: dir}

	_, err := e.Run(ctx, in, func(domain.Finding) {})
	if err == nil {
		t.Fatal("Run() error = nil, want an error — docreview has no deterministic fallback for a missing AI service")
	}
}

func TestEngine_ID(t *testing.T) {
	if got := New(nil).ID(); got != domain.EngineDocReview {
		t.Errorf("ID() = %q, want %q", got, domain.EngineDocReview)
	}
}
