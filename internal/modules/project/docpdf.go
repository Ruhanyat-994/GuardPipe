package project

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// PDFTextExtractor pulls the plain text out of a PDF's bytes — the seam
// that lets UploadDocument's PDF path be tested without depending on the
// real parser's internals (CLAUDE.md: no mocking framework, hand-written
// fakes only).
type PDFTextExtractor interface {
	ExtractText(content []byte) (string, error)
}

// ledongthucPDFExtractor is the production PDFTextExtractor, backed by
// github.com/ledongthuc/pdf — pure Go, MIT-licensed, no cgo or system
// dependency, chosen specifically because it does nothing beyond plain-text
// extraction (CLAUDE.md: "casual dependencies are off-brand"). The original
// PDF binary is never stored — Document.Content ends up holding the
// extracted text, same as every other document type (see UploadDocument).
type ledongthucPDFExtractor struct{}

// NewPDFTextExtractor builds the production PDFTextExtractor.
func NewPDFTextExtractor() PDFTextExtractor {
	return ledongthucPDFExtractor{}
}

func (ledongthucPDFExtractor) ExtractText(content []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("open PDF: %w", err)
	}
	textReader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract PDF text: %w", err)
	}
	text, err := io.ReadAll(textReader)
	if err != nil {
		return "", fmt.Errorf("read PDF text: %w", err)
	}
	return string(text), nil
}
