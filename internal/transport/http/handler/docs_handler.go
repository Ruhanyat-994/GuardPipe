package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/api"
)

// DocsHandler serves the machine-readable OpenAPI document
// (documentation/07-api-specification.md §12). It never returns HTML
// (documentation/07-api-specification.md §11: "the backend never returns
// HTML, ever") — an interactive Swagger/Redoc UI is a developer-local
// concern (see the `docs` Makefile target), not something this binary
// serves.
type DocsHandler struct{}

func NewDocsHandler() *DocsHandler {
	return &DocsHandler{}
}

// OpenAPISpec handles `GET /api/v1/openapi.yaml`.
func (h *DocsHandler) OpenAPISpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", api.OpenAPISpec)
}
