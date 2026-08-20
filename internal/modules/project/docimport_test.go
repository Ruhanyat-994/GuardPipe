package project_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
)

func TestParseGoogleDocLink_AcceptsSupportedShapes(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantExport string
		wantPrefix string
	}{
		{
			name:       "docs edit link with sharing query",
			url:        "https://docs.google.com/document/d/1AbC-xyz_123/edit?usp=sharing",
			wantExport: "https://docs.google.com/document/d/1AbC-xyz_123/export?format=txt",
			wantPrefix: "google-doc-",
		},
		{
			name:       "docs link with a fragment",
			url:        "https://docs.google.com/document/d/1AbC-xyz_123/edit#heading=h.abc123",
			wantExport: "https://docs.google.com/document/d/1AbC-xyz_123/export?format=txt",
			wantPrefix: "google-doc-",
		},
		{
			name:       "drive file view link",
			url:        "https://drive.google.com/file/d/1AbC-xyz_123/view?usp=sharing",
			wantExport: "https://drive.google.com/uc?export=download&id=1AbC-xyz_123",
			wantPrefix: "google-drive-",
		},
		{
			name:       "drive open?id= link",
			url:        "https://drive.google.com/open?id=1AbC-xyz_123",
			wantExport: "https://drive.google.com/uc?export=download&id=1AbC-xyz_123",
			wantPrefix: "google-drive-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exportURL, filename, err := project.ParseGoogleDocLink(tt.url)
			require.NoError(t, err)
			require.Equal(t, tt.wantExport, exportURL)
			require.True(t, strings.HasPrefix(filename, tt.wantPrefix), "filename %q should start with %q", filename, tt.wantPrefix)
			require.True(t, strings.HasSuffix(filename, ".txt"), "filename %q should end in .txt", filename)
		})
	}
}

// TestParseGoogleDocLink_RejectsUnsupportedShapes is the near-miss half —
// every one of these must fail with a clear reason rather than being
// silently mangled into a broken export URL.
func TestParseGoogleDocLink_RejectsUnsupportedShapes(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"non-google host", "https://evil.example.com/document/d/1AbC-xyz_123/edit"},
		{"plain http, not https", "http://docs.google.com/document/d/1AbC-xyz_123/edit"},
		{"google sheets, not docs", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit"},
		{"google slides, not docs", "https://docs.google.com/presentation/d/1AbC-xyz_123/edit"},
		{"drive folder, not a file", "https://drive.google.com/drive/folders/1AbC-xyz_123"},
		{"drive link with no id", "https://drive.google.com/open"},
		{"malformed url", "not a url at all"},
		{"empty string", ""},
		{"bare host, no path", "https://docs.google.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := project.ParseGoogleDocLink(tt.url)
			require.Error(t, err)
		})
	}
}

func TestHTTPURLFetcher_ReturnsContentTypeAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, _ = w.Write([]byte("hello from a fake export endpoint"))
	}))
	defer srv.Close()

	fetcher := project.NewHTTPURLFetcher()
	contentType, body, err := fetcher.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Contains(t, contentType, "text/plain")
	require.Equal(t, "hello from a fake export endpoint", string(body))
}

func TestHTTPURLFetcher_CapsResponseSize(t *testing.T) {
	huge := strings.Repeat("x", 200*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	fetcher := project.NewHTTPURLFetcher()
	_, body, err := fetcher.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Less(t, len(body), len(huge), "fetcher should cap the read, not buffer the whole body")
}

func TestHTTPURLFetcher_RejectsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fetcher := project.NewHTTPURLFetcher()
	_, _, err := fetcher.Fetch(context.Background(), srv.URL)
	require.Error(t, err)
}
