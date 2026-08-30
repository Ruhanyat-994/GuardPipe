package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
}

func decodeProblem(t *testing.T, body *bytes.Buffer) apperrors.ProblemDetails {
	t.Helper()
	var pd apperrors.ProblemDetails
	if err := json.Unmarshal(body.Bytes(), &pd); err != nil {
		t.Fatalf("response is not a valid ProblemDetails: %v\n%s", err, body.String())
	}
	return pd
}

// --- RequestID ---

func TestRequestID_GeneratesOneWhenAbsent(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Header().Get(middleware.RequestIDHeader) == "" {
		t.Error("no X-Request-ID header on the response")
	}
}

func TestRequestID_PropagatesAnExistingOne(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.RequestIDHeader, "client-supplied-id")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(middleware.RequestIDHeader); got != "client-supplied-id" {
		t.Errorf("X-Request-ID = %q, want the client-supplied value", got)
	}
}

// --- ErrorMapper ---

func TestErrorMapper_RendersTypedErrorAsProblemDetails(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorMapper())
	r.GET("/", func(c *gin.Context) {
		c.Error(apperrors.NotFound("project.not_found", "no such project"))
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	pd := decodeProblem(t, rec.Body)
	if pd.Code != "project.not_found" {
		t.Errorf("Code = %q, want %q", pd.Code, "project.not_found")
	}
}

// TestErrorMapper_BareErrorIsTreatedAsInternal is the fail-safe case: a
// handler that returns a plain error (forgot to wrap it) must still produce
// a generic 500, never leak the raw error string.
func TestErrorMapper_BareErrorIsTreatedAsInternal(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorMapper())
	r.GET("/", func(c *gin.Context) {
		c.Error(errors.New("dial tcp 10.0.0.5:5432: connect: connection refused"))
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if bytesContainsString(rec.Body.Bytes(), "10.0.0.5") {
		t.Fatalf("response leaked the bare error's detail: %s", rec.Body.String())
	}
}

func TestErrorMapper_NoErrorLeavesResponseUntouched(t *testing.T) {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (ErrorMapper must not interfere with a successful response)", rec.Code)
	}
}

// --- Recovery ---

func TestRecovery_CatchesPanicAndRenders500(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recovery(newTestLogger()))
	r.GET("/", func(c *gin.Context) {
		panic("something went very wrong")
	})

	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("panic escaped Recovery middleware: %v", p)
			}
		}()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if bytesContainsString(rec.Body.Bytes(), "something went very wrong") {
		t.Fatalf("response leaked the panic value: %s", rec.Body.String())
	}
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(middleware.Recovery(newTestLogger()))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// --- CORS ---

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	r := gin.New()
	r.Use(middleware.CORS([]string{"https://app.example.com"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the allowed origin", got)
	}
}

// TestCORS_RejectsUnlistedOrigin is the near-miss: an origin not on the
// allowlist must not get the header at all.
func TestCORS_RejectsUnlistedOrigin(t *testing.T) {
	r := gin.New()
	r.Use(middleware.CORS([]string{"https://app.example.com"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for an unlisted origin", got)
	}
}

func TestCORS_HandlesPreflight(t *testing.T) {
	r := gin.New()
	r.Use(middleware.CORS([]string{"https://app.example.com"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
}

// TestCORS_AllowsPUT guards the `PUT /projects/{id}/credential` endpoint
// (the private-repo PAT flow) — PUT was missing from Access-Control-Allow-Methods,
// so a browser's preflight silently blocked the actual request even though
// the handler itself worked fine (curl doesn't preflight, so that path was
// never caught without a real browser).
func TestCORS_AllowsPUT(t *testing.T) {
	r := gin.New()
	r.Use(middleware.CORS([]string{"https://app.example.com"}))
	r.PUT("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	r.ServeHTTP(rec, req)

	allowed := rec.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(allowed, "PUT") {
		t.Errorf("Access-Control-Allow-Methods = %q, want it to include PUT", allowed)
	}
}

// --- SecurityHeaders ---

func TestSecurityHeaders_SetsExpectedHeaders(t *testing.T) {
	r := gin.New()
	r.Use(middleware.SecurityHeaders())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, header := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy"} {
		if rec.Header().Get(header) == "" {
			t.Errorf("missing security header %q", header)
		}
	}
}

// --- RateLimit ---

func TestRateLimit_AllowsUpToTheLimit(t *testing.T) {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.RateLimit(3, time.Minute), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := range 3 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.5:1234"
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rec.Code)
		}
	}
}

func TestRateLimit_RejectsOverTheLimit(t *testing.T) {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.RateLimit(2, time.Minute), func(c *gin.Context) { c.Status(http.StatusOK) })

	for range 2 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		r.ServeHTTP(rec, req)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on a 429")
	}
}

// TestRateLimit_TracksClientsIndependently is the near-miss: one IP being
// rate-limited must not affect a different IP.
func TestRateLimit_TracksClientsIndependently(t *testing.T) {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.RateLimit(1, time.Minute), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "203.0.113.1:1"
	r.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first client: status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "203.0.113.2:1"
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second (different) client: status = %d, want 200, not rate-limited by the first client's usage", rec2.Code)
	}
}

// --- Auth / RBAC ---

// fakeIdentityService is a hand-written fake implementing the full
// identity.Service interface — only Verify has real behaviour, since that's
// all Auth middleware calls; the rest exist only to satisfy the interface.
type fakeIdentityService struct {
	claims        *identity.Claims
	err           error
	suspensionErr error
}

func (f fakeIdentityService) Register(context.Context, identity.RegisterInput) (*identity.User, error) {
	return nil, nil
}
func (f fakeIdentityService) Login(context.Context, string, string) (*identity.TokenPair, error) {
	return nil, nil
}
func (f fakeIdentityService) Refresh(context.Context, string) (*identity.TokenPair, error) {
	return nil, nil
}
func (f fakeIdentityService) Logout(context.Context, string) error { return nil }
func (f fakeIdentityService) Verify(context.Context, string) (*identity.Claims, error) {
	return f.claims, f.err
}
func (f fakeIdentityService) CheckSuspension(context.Context, domain.Actor) error { return f.suspensionErr }
func (f fakeIdentityService) Me(context.Context, domain.Actor) (*identity.User, error) {
	return nil, nil
}

func TestAuth_ValidBearerTokenSetsActor(t *testing.T) {
	wantActor := domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember}
	svc := fakeIdentityService{claims: &identity.Claims{UserID: wantActor.UserID, OrgID: wantActor.OrgID, Role: wantActor.Role}}

	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), func(c *gin.Context) {
		actor, ok := middleware.ActorFromContext(c)
		if !ok {
			t.Error("ActorFromContext() ok = false after Auth ran")
		}
		if actor.UserID != wantActor.UserID {
			t.Errorf("actor.UserID = %v, want %v", actor.UserID, wantActor.UserID)
		}
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAuth_MissingBearerPrefixIsRejected(t *testing.T) {
	svc := fakeIdentityService{}
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "valid-token-without-bearer-prefix")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_InvalidTokenIsRejected(t *testing.T) {
	svc := fakeIdentityService{err: apperrors.Unauthorized("auth.token_invalid", "bad token")}
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRBAC_AllowsMatchingRole(t *testing.T) {
	svc := fakeIdentityService{claims: &identity.Claims{UserID: id.New(), OrgID: id.New(), Role: domain.RoleAdmin}}
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), middleware.RBAC(domain.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a matching role", rec.Code)
	}
}

// TestRBAC_RejectsNonMatchingRole is the near-miss: a viewer hitting an
// admin-only route must get 403, not through.
func TestRBAC_RejectsNonMatchingRole(t *testing.T) {
	svc := fakeIdentityService{claims: &identity.Claims{UserID: id.New(), OrgID: id.New(), Role: domain.RoleViewer}}
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), middleware.RBAC(domain.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a non-matching role", rec.Code)
	}
}

func bytesContainsString(b []byte, s string) bool {
	return len(s) > 0 && bytes.Contains(b, []byte(s))
}

// --- SuspensionCheck (BUILD_GUIDE.md Phase 14) ---

func TestSuspensionCheck_AllowsActiveAccount(t *testing.T) {
	svc := fakeIdentityService{claims: &identity.Claims{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember}}
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), middleware.SuspensionCheck(svc), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an active account", rec.Code)
	}
}

// TestSuspensionCheck_RejectsSuspendedAccount is the near-miss: a valid,
// unexpired access token for a since-suspended account must still be
// rejected on the very next request, not just at its next login.
func TestSuspensionCheck_RejectsSuspendedAccount(t *testing.T) {
	svc := fakeIdentityService{
		claims:        &identity.Claims{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember},
		suspensionErr: apperrors.Forbidden("auth.account_suspended", "this account has been suspended"),
	}
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(svc), middleware.SuspensionCheck(svc), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a suspended account", rec.Code)
	}
}

// --- RequirePlatformOperator (BUILD_GUIDE.md Phase 14) ---

type fakeOperatorChecker struct {
	operators map[uuid.UUID]bool
	err       error
}

func (f fakeOperatorChecker) IsOperator(_ context.Context, userID uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.operators[userID], nil
}

func TestRequirePlatformOperator_AllowsOperator(t *testing.T) {
	userID := id.New()
	authSvc := fakeIdentityService{claims: &identity.Claims{UserID: userID, OrgID: id.New(), Role: domain.RoleMember}}
	opSvc := fakeOperatorChecker{operators: map[uuid.UUID]bool{userID: true}}

	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(authSvc), middleware.RequirePlatformOperator(opSvc), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a platform operator", rec.Code)
	}
}

// TestRequirePlatformOperator_RejectsNonOperator is the near-miss: even an
// org's own domain.RoleAdmin — a real role, just the wrong one — must be
// rejected here. Being an org admin never implies platform-operator access
// (see the package's own doc comment on why these two are never conflated).
func TestRequirePlatformOperator_RejectsNonOperator(t *testing.T) {
	userID := id.New()
	authSvc := fakeIdentityService{claims: &identity.Claims{UserID: userID, OrgID: id.New(), Role: domain.RoleAdmin}}
	opSvc := fakeOperatorChecker{operators: map[uuid.UUID]bool{}}

	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.GET("/", middleware.Auth(authSvc), middleware.RequirePlatformOperator(opSvc), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for an org-admin who isn't a platform operator", rec.Code)
	}
}
