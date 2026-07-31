package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeIdentityService is a hand-written fake (no mocking framework) that
// lets each test script exactly the return value each identity.Service
// method should produce, independent of any real business logic — the
// business logic itself is already covered by
// internal/modules/identity/service_test.go. These tests are about the
// HTTP layer: binding, validation, status codes, cookies, and error
// mapping.
type fakeIdentityService struct {
	registerUser *identity.User
	registerErr  error
	loginPair    *identity.TokenPair
	loginErr     error
	refreshPair  *identity.TokenPair
	refreshErr   error
	logoutErr    error
	meUser       *identity.User
	meErr        error
}

func (f *fakeIdentityService) Register(context.Context, identity.RegisterInput) (*identity.User, error) {
	return f.registerUser, f.registerErr
}
func (f *fakeIdentityService) Login(context.Context, string, string) (*identity.TokenPair, error) {
	return f.loginPair, f.loginErr
}
func (f *fakeIdentityService) Refresh(context.Context, string) (*identity.TokenPair, error) {
	return f.refreshPair, f.refreshErr
}
func (f *fakeIdentityService) Logout(context.Context, string) error { return f.logoutErr }
func (f *fakeIdentityService) Verify(context.Context, string) (*identity.Claims, error) {
	return nil, nil
}
func (f *fakeIdentityService) Me(context.Context, domain.Actor) (*identity.User, error) {
	return f.meUser, f.meErr
}

func newRouter(svc identity.Service) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	v := validate.New()
	h := handler.NewAuthHandler(svc, v, false, 7*24*time.Hour)

	r.POST("/register", h.Register)
	r.POST("/login", h.Login)
	r.POST("/refresh", h.Refresh)
	// Logout/Me need an Actor in context, same as a real Auth-protected
	// route would provide.
	authed := r.Group("/")
	authed.Use(func(c *gin.Context) {
		c.Set("actor", domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember})
		c.Next()
	})
	authed.POST("/logout", h.Logout)
	authed.GET("/me", h.Me)

	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRegister_ValidRequestReturns201(t *testing.T) {
	svc := &fakeIdentityService{registerUser: &identity.User{
		ID: id.New(), Email: "nadia@example.com", DisplayName: "Nadia", Role: domain.RoleAdmin, CreatedAt: time.Now(),
	}}
	r := newRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/register", map[string]string{
		"email": "nadia@example.com", "display_name": "Nadia", "password": "correct-horse-battery",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got["email"] != "nadia@example.com" {
		t.Errorf("email = %v, want nadia@example.com", got["email"])
	}
	if _, present := got["password_hash"]; present {
		t.Error("response leaked password_hash — a domain/internal field must never reach the wire")
	}
}

func TestRegister_MissingFieldsReturns400WithFieldErrors(t *testing.T) {
	svc := &fakeIdentityService{}
	r := newRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/register", map[string]string{"email": "not-an-email"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
	var pd apperrors.ProblemDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &pd); err != nil {
		t.Fatalf("response is not a ProblemDetails: %v", err)
	}
	if len(pd.Errors) == 0 {
		t.Error("expected field errors in the validation response")
	}
}

func TestRegister_ServiceConflictPropagatesAs409(t *testing.T) {
	svc := &fakeIdentityService{registerErr: apperrors.Conflict("identity.email_taken", "already exists")}
	r := newRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/register", map[string]string{
		"email": "nadia@example.com", "display_name": "Nadia", "password": "correct-horse-battery",
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_ValidCredentialsSetsRefreshCookieAndReturnsAccessToken(t *testing.T) {
	svc := &fakeIdentityService{loginPair: &identity.TokenPair{
		AccessToken: "access-token-value", RefreshToken: "refresh-token-value", ExpiresIn: 900,
		User: &identity.User{ID: id.New(), Email: "nadia@example.com", DisplayName: "Nadia", Role: domain.RoleMember},
	}}
	r := newRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/login", map[string]string{"email": "nadia@example.com", "password": "x"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got["access_token"] != "access-token-value" {
		t.Errorf("access_token = %v, want access-token-value", got["access_token"])
	}
	if _, present := got["refresh_token"]; present {
		t.Error("refresh_token must never appear in the JSON body — it's cookie-only")
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, ck := range cookies {
		if ck.Name == "gp_refresh" {
			found = true
			if !ck.HttpOnly {
				t.Error("gp_refresh cookie is not HttpOnly")
			}
			if ck.Value != "refresh-token-value" {
				t.Errorf("gp_refresh cookie value = %q, want refresh-token-value", ck.Value)
			}
		}
	}
	if !found {
		t.Error("no gp_refresh cookie set on a successful login")
	}
}

func TestLogin_InvalidCredentialsReturns401(t *testing.T) {
	svc := &fakeIdentityService{loginErr: apperrors.Unauthorized("auth.invalid_credentials", "wrong")}
	r := newRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/login", map[string]string{"email": "nadia@example.com", "password": "wrong"})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRefresh_NoCookiePresentReturns401(t *testing.T) {
	svc := &fakeIdentityService{}
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRefresh_ValidCookieRotatesAndReturnsNewAccessToken(t *testing.T) {
	svc := &fakeIdentityService{refreshPair: &identity.TokenPair{
		AccessToken: "new-access-token", RefreshToken: "new-refresh-token", ExpiresIn: 900,
	}}
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "gp_refresh", Value: "old-refresh-token"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "gp_refresh" && ck.Value != "new-refresh-token" {
			t.Errorf("gp_refresh cookie = %q, want the rotated value", ck.Value)
		}
	}
}

func TestLogout_AlwaysReturns204(t *testing.T) {
	svc := &fakeIdentityService{}
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body: %s", rec.Code, rec.Body.String())
	}
}

func TestMe_ReturnsCurrentUser(t *testing.T) {
	svc := &fakeIdentityService{meUser: &identity.User{
		ID: id.New(), Email: "nadia@example.com", DisplayName: "Nadia", Role: domain.RoleMember,
	}}
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got["email"] != "nadia@example.com" {
		t.Errorf("email = %v, want nadia@example.com", got["email"])
	}
}
