package sonarqube_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sonarqube"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(body))
}

func TestClient_GetTask_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		writeJSON(t, w, http.StatusOK, map[string]any{
			"task": map[string]any{"status": "SUCCESS", "analysisId": "AY_analysis-1"},
		})
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "token-123", nil)
	task, err := client.GetTask(context.Background(), "task-1")
	require.NoError(t, err)
	require.Equal(t, "/api/ce/task", gotPath)
	require.Equal(t, "id=task-1", gotQuery)
	require.Equal(t, sonarqube.TaskSuccess, task.Status)
	require.Equal(t, "AY_analysis-1", task.AnalysisID)
}

func TestClient_GetTask_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "bad-token", nil)
	_, err := client.GetTask(context.Background(), "task-1")
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "sonarqube.unauthorized", appErr.Code)
}

func TestClient_SearchIssues_PaginatesAndMapsTextRange(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/api/issues/search", r.URL.Path)
		require.Equal(t, "VULNERABILITY", r.URL.Query().Get("types"))
		if r.URL.Query().Get("p") == "1" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"issues": []map[string]any{
					{
						"key": "issue-1", "rule": "go:S2076", "severity": "CRITICAL",
						"component": "proj:main.go", "message": "SQL injection",
						"textRange": map[string]any{"startLine": 10, "endLine": 12},
					},
				},
				"paging": map[string]any{"pageIndex": 1, "pageSize": 1, "total": 2},
			})
			return
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"issues": []map[string]any{
				{"key": "issue-2", "rule": "go:S1234", "severity": "MAJOR", "component": "proj:util.go", "line": 5},
			},
			"paging": map[string]any{"pageIndex": 2, "pageSize": 1, "total": 2},
		})
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "token", nil)
	issues, err := client.SearchIssues(context.Background(), "proj")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, issues, 2)
	require.Equal(t, "issue-1", issues[0].Key)
	require.Equal(t, 10, issues[0].Line)
	require.Equal(t, 12, issues[0].LineEnd)
	require.Equal(t, "issue-2", issues[1].Key)
	require.Equal(t, 5, issues[1].Line)
}

func TestClient_SearchHotspots_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/hotspots/search", r.URL.Path)
		writeJSON(t, w, http.StatusOK, map[string]any{
			"hotspots": []map[string]any{
				{"key": "hs-1", "ruleKey": "go:S5332", "component": "proj:server.go", "line": 42, "vulnerabilityProbability": "HIGH", "message": "Insecure protocol"},
			},
			"paging": map[string]any{"pageIndex": 1, "pageSize": 500, "total": 1},
		})
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "token", nil)
	hotspots, err := client.SearchHotspots(context.Background(), "proj")
	require.NoError(t, err)
	require.Len(t, hotspots, 1)
	require.Equal(t, "HIGH", hotspots[0].VulnerabilityProbability)
	require.Equal(t, "go:S5332", hotspots[0].RuleKey)
}

func TestClient_GetRule_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/rules/show", r.URL.Path)
		require.Equal(t, "go:S2076", r.URL.Query().Get("key"))
		writeJSON(t, w, http.StatusOK, map[string]any{
			"rule": map[string]any{
				"key": "go:S2076", "name": "OS commands should not be vulnerable to injection attacks",
				"htmlDesc": "<p>Use a parameterised API instead.</p>", "cwe": []string{"CWE-78"},
			},
		})
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "token", nil)
	rule, err := client.GetRule(context.Background(), "go:S2076")
	require.NoError(t, err)
	require.Equal(t, "go:S2076", rule.Key)
	require.Contains(t, rule.RemediationHTML, "parameterised")
	require.Equal(t, []string{"CWE-78"}, rule.CWE)
}

func TestClient_QualityGateStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "analysisId=AY_1", r.URL.RawQuery)
		writeJSON(t, w, http.StatusOK, map[string]any{
			"projectStatus": map[string]any{"status": "OK"},
		})
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "token", nil)
	status, err := client.QualityGateStatus(context.Background(), "AY_1")
	require.NoError(t, err)
	require.Equal(t, "OK", status)
}

func TestClient_Unreachable(t *testing.T) {
	client := sonarqube.NewClient("http://127.0.0.1:1", "token", nil)
	_, err := client.GetTask(context.Background(), "task-1")
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "sonarqube.unreachable", appErr.Code)
}

func TestClient_UpstreamServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	client := sonarqube.NewClient(srv.URL, "token", nil)
	_, err := client.GetTask(context.Background(), "task-1")
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "sonarqube.upstream_error", appErr.Code)
}
