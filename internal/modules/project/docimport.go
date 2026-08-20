package project

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// URLFetcher fetches a URL's body — the seam that lets ImportDocumentFromURL
// be tested without a live network call (CLAUDE.md: "no mocking framework,
// hand-written fakes only. Tests never call ... live").
type URLFetcher interface {
	Fetch(ctx context.Context, target string) (contentType string, body []byte, err error)
}

// maxFetchBytes caps how much of a remote response is ever read into
// memory. One byte over the document size limit is enough for
// createDocument's existing "too large" check to reject it — there's no
// need to risk an unbounded download for a link that turns out not to be a
// small text document.
const maxFetchBytes = maxDocumentSizeBytes + 1

// httpURLFetcher is the production URLFetcher.
type httpURLFetcher struct {
	client *http.Client
}

// NewHTTPURLFetcher builds the production URLFetcher used to import a
// document from a Google Docs/Drive share link (main.go's wiring).
func NewHTTPURLFetcher() URLFetcher {
	return &httpURLFetcher{client: &http.Client{Timeout: 10 * time.Second}}
}

func (f *httpURLFetcher) Fetch(ctx context.Context, target string) (string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", nil, fmt.Errorf("read response: %w", err)
	}
	return resp.Header.Get("Content-Type"), body, nil
}

var (
	googleDocsPathPattern  = regexp.MustCompile(`^/document/d/([a-zA-Z0-9_-]+)`)
	googleDriveFilePattern = regexp.MustCompile(`^/file/d/([a-zA-Z0-9_-]+)`)
)

// ParseGoogleDocLink accepts a Google Docs or Google Drive share link and
// returns the URL to fetch its plain-text export from, plus a filename
// synthesized from the file ID — Google's export response doesn't reliably
// name the file, and the caller (createDocument) needs something ending in
// an allowed extension, which ".txt" always is.
//
// Only Google Docs documents and generic Drive files are supported —
// Sheets/Slides/Forms/folders don't have a plain-text export and are
// rejected with a clear message rather than silently mangled.
func ParseGoogleDocLink(rawURL string) (exportURL, filename string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("not a valid URL")
	}
	if u.Scheme != "https" {
		return "", "", fmt.Errorf("must be an https:// link")
	}

	switch strings.ToLower(u.Host) {
	case "docs.google.com":
		m := googleDocsPathPattern.FindStringSubmatch(u.Path)
		if m == nil {
			return "", "", fmt.Errorf("only Google Docs document links (docs.google.com/document/d/...) are supported — Sheets, Slides, and Forms aren't")
		}
		docID := m[1]
		return "https://docs.google.com/document/d/" + docID + "/export?format=txt",
			"google-doc-" + shortDocID(docID) + ".txt", nil

	case "drive.google.com":
		if m := googleDriveFilePattern.FindStringSubmatch(u.Path); m != nil {
			docID := m[1]
			return "https://drive.google.com/uc?export=download&id=" + docID,
				"google-drive-" + shortDocID(docID) + ".txt", nil
		}
		if docID := u.Query().Get("id"); docID != "" {
			return "https://drive.google.com/uc?export=download&id=" + docID,
				"google-drive-" + shortDocID(docID) + ".txt", nil
		}
		return "", "", fmt.Errorf("couldn't find a file ID in that Google Drive link")

	default:
		return "", "", fmt.Errorf("must be a docs.google.com or drive.google.com link")
	}
}

func shortDocID(docID string) string {
	if len(docID) > 8 {
		return docID[:8]
	}
	return docID
}
