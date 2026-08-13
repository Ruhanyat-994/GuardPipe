package osv_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/osv"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(body))
}

func TestClient_QueryBatch_Success(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"vulns": []map[string]any{{"id": "GHSA-1234"}, {"id": "CVE-2024-9999"}}},
				{"vulns": []map[string]any{}},
			},
		})
	}))
	defer srv.Close()

	client := osv.NewClient(srv.URL, nil)
	ids, err := client.QueryBatch(context.Background(), []osv.PackageQuery{
		{Ecosystem: "npm", Name: "left-pad", Version: "1.0.0"},
		{Ecosystem: "npm", Name: "clean-pkg", Version: "2.0.0"},
	})
	require.NoError(t, err)
	require.Equal(t, "/v1/querybatch", gotPath)
	require.Contains(t, gotBody, "left-pad")
	require.Equal(t, [][]string{{"GHSA-1234", "CVE-2024-9999"}, {}}, ids)
}

func TestClient_QueryBatch_Empty(t *testing.T) {
	client := osv.NewClient("http://example.invalid", nil)
	ids, err := client.QueryBatch(context.Background(), nil)
	require.NoError(t, err)
	require.Nil(t, ids)
}

func TestClient_QueryBatch_ExceedsMaxBatchSize(t *testing.T) {
	client := osv.NewClient("http://example.invalid", nil)
	queries := make([]osv.PackageQuery, osv.MaxBatchSize+1)
	_, err := client.QueryBatch(context.Background(), queries)
	require.Error(t, err)
}

func TestClient_GetVulnerability_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/vulns/GHSA-1234", r.URL.Path)
		writeJSON(t, w, http.StatusOK, map[string]any{
			"id":      "GHSA-1234",
			"summary": "left-pad has a bug",
			"aliases": []string{"CVE-2024-9999"},
			"severity": []map[string]any{
				{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
			},
			"affected": []map[string]any{
				{
					"package": map[string]any{"ecosystem": "npm", "name": "left-pad"},
					"ranges": []map[string]any{
						{"type": "SEMVER", "events": []map[string]any{{"introduced": "0"}, {"fixed": "1.0.1"}}},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := osv.NewClient(srv.URL, nil)
	v, err := client.GetVulnerability(context.Background(), "GHSA-1234")
	require.NoError(t, err)
	require.Equal(t, "GHSA-1234", v.ID)
	require.Equal(t, []string{"CVE-2024-9999"}, v.Aliases)
	require.Len(t, v.Affected, 1)
	require.Equal(t, "1.0.1", v.Affected[0].Ranges[0].Events[1].Fixed)
}

func TestClient_GetVulnerability_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := osv.NewClient(srv.URL, nil)
	_, err := client.GetVulnerability(context.Background(), "does-not-exist")
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestClient_QueryBatch_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := osv.NewClient(srv.URL, nil)
	_, err := client.QueryBatch(context.Background(), []osv.PackageQuery{{Ecosystem: "npm", Name: "x", Version: "1"}})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindExternal, appErr.Kind)
}
