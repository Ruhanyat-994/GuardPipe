package docreview

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

var markdownHeading = regexp.MustCompile(`(?m)^#{1,6}\s+.+$`)

// headingContext extracts every Markdown heading in content for
// PromptReviewDocument's "heading_context" var (modules/ai/prompts.go) —
// location attribution for a non-Markdown document (no headings match)
// degrades to a plain "no headings found" note, not an error.
func headingContext(content string) string {
	matches := markdownHeading.FindAllString(content, -1)
	if len(matches) == 0 {
		return "(no headings found)"
	}
	return strings.Join(matches, " | ")
}

// categoriesVar renders categoryRuleIDs (rules.go) for PromptReviewDocument's
// "categories" var — the exact list the model is told not to invent beyond
// (modules/ai/prompts.go's own template text: "Do not invent a category
// outside the supplied list").
func categoriesVar() string {
	return strings.Join(categoryRuleIDs, ", ")
}

// runAIPass reviews every document — uploaded (in.Documents) first, then
// repo-discovered — combined and capped at maxDocumentsPerScan
// (documentation/05-module-specifications.md §11's own cap: 20 per scan).
// Uploaded documents take priority when the combined set exceeds the cap:
// a user explicitly uploading a document is a stronger signal of intent
// than a file merely existing in one of the matched repository directories.
func runAIPass(ctx context.Context, aiSvc ai.Service, in domain.ScanInput, repoDocuments []discoveredDocument, emit func(domain.Finding)) (filesReviewed int, err error) {
	all := make([]discoveredDocument, 0, len(in.Documents)+len(repoDocuments))
	for _, d := range in.Documents {
		all = append(all, discoveredDocument{Path: d.Path, Content: d.Content})
	}
	all = append(all, repoDocuments...)
	if len(all) > maxDocumentsPerScan {
		all = all[:maxDocumentsPerScan]
	}

	for _, doc := range all {
		if ctx.Err() != nil {
			return filesReviewed, ctx.Err()
		}
		if err := reviewOneDocument(ctx, aiSvc, in, doc, emit); err != nil {
			return filesReviewed, err
		}
		filesReviewed++
	}
	return filesReviewed, nil
}

func reviewOneDocument(ctx context.Context, aiSvc ai.Service, in domain.ScanInput, doc discoveredDocument, emit func(domain.Finding)) error {
	source := domain.Location{Type: domain.LocationTypeFile, Path: doc.Path}

	result, err := aiSvc.Run(ctx, ai.RunInput{
		PromptID: ai.PromptReviewDocument,
		Vars: map[string]string{
			"document_path":   doc.Path,
			"heading_context": headingContext(doc.Content),
			"categories":      categoriesVar(),
		},
		Untrusted: []ai.UntrustedBlock{{Label: doc.Path, Content: doc.Content}},
		ScanID:    in.ScanID,
		Engine:    domain.EngineDocReview,
		Source:    source,
	}, emit) // emit is also where ai.Service raises prompt_injection_attempt itself, on a discarded response
	if err != nil {
		return err
	}
	if result.Discarded {
		return nil // injection finding already emitted by ai.Service
	}

	findings, ok := result.Value.(ai.DocumentReviewResponse)
	if !ok {
		return fmt.Errorf("docreview: unexpected AI result type %T", result.Value)
	}
	for _, f := range findings {
		meta, ok := classifyRuleID(f.RuleID)
		if !ok {
			continue // the model named a rule_id outside the supplied category list — dropped, not fabricated into a fake match
		}
		emit(aiFinding(in.ScanID, doc.Path, meta, f))
	}
	return nil
}
