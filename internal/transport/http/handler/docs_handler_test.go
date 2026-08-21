package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
)

func TestDocsHandler_OpenAPISpec(t *testing.T) {
	r := gin.New()
	h := handler.NewDocsHandler()
	r.GET("/openapi.yaml", h.OpenAPISpec)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/yaml")
	require.True(t, strings.HasPrefix(rec.Body.String(), "openapi: 3.1.0"))
	require.Contains(t, rec.Body.String(), "/api/v1/auth/login")
}
