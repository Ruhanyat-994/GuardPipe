package project_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
)

// buildMinimalPDF hand-assembles the smallest valid single-page PDF that
// contains real, extractable text — one Helvetica text run — computing each
// object's byte offset as it writes, so the xref table it emits is always
// correct regardless of the text's length. This is a real PDF exercised
// through the real github.com/ledongthuc/pdf parser, not a fabricated
// binary blob standing in for one.
func buildMinimalPDF(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	var offsets [6]int

	buf.WriteString("%PDF-1.4\n")

	writeObj := func(n int, body string) {
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "+
		"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	writeObj(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	content := fmt.Sprintf("BT /F1 24 Tf 100 700 Td (%s) Tj ET", text)
	writeObj(5, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))

	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 6\n")
	buf.WriteString("0000000000 65535 f \n")
	for n := 1; n <= 5; n++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[n])
	}
	buf.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF", xrefOffset)

	return buf.Bytes()
}

func TestPDFTextExtractor_ExtractsRealText(t *testing.T) {
	pdfBytes := buildMinimalPDF(t, "Hello docreview")

	extractor := project.NewPDFTextExtractor()
	text, err := extractor.ExtractText(pdfBytes)
	require.NoError(t, err)
	require.Contains(t, text, "Hello docreview")
}

// TestPDFTextExtractor_RejectsNonPDFBytes is the near-miss half — a file
// with a .pdf extension but garbage content must fail extraction, not
// silently produce empty or garbled "text".
func TestPDFTextExtractor_RejectsNonPDFBytes(t *testing.T) {
	extractor := project.NewPDFTextExtractor()
	_, err := extractor.ExtractText([]byte("this is not a PDF at all"))
	require.Error(t, err)
}

func TestPDFTextExtractor_RejectsEmptyInput(t *testing.T) {
	extractor := project.NewPDFTextExtractor()
	_, err := extractor.ExtractText(nil)
	require.Error(t, err)
}
