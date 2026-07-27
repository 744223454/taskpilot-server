package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/744223454/taskpilot-server/internal/config"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/gin-gonic/gin"
)

type fakeRefreshSessionStore struct {
	session jwtauth.RefreshSession
	revoked bool
}

func (s *fakeRefreshSessionStore) Create(context.Context, jwtauth.RefreshSession, string) error {
	return nil
}

func (s *fakeRefreshSessionStore) Rotate(context.Context, string, string, string, time.Time) (jwtauth.RefreshSession, error) {
	return s.session, nil
}

func (s *fakeRefreshSessionStore) Revoke(context.Context, string, string) error {
	s.revoked = true
	return nil
}

func TestRefreshHandlerSetsRotatedHttpOnlyCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := jwtauth.NewManager("test-secret", 900)
	refreshToken, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	store := &fakeRefreshSessionStore{session: jwtauth.RefreshSession{
		ID:        refreshToken.SessionID,
		UserID:    42,
		Email:     "user@example.com",
		Nickname:  "tester",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}}
	appConfig := config.Config{}
	appConfig.Auth.AccessSecret = "test-secret"
	appConfig.Auth.AccessExpire = 900
	appConfig.Auth.RefreshExpire = 3600
	appConfig.Auth.CookieSecure = true
	svcCtx := &svc.ServiceContext{Config: appConfig, JWT: manager, RefreshSessions: store}
	router := gin.New()
	router.POST("/api/v1/auth/refresh", middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), RefreshHandler(svcCtx))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	request.AddCookie(&http.Cookie{Name: jwtauth.RefreshCookieName, Value: refreshToken.Raw})
	request.AddCookie(&http.Cookie{Name: jwtauth.CSRFCookieName, Value: "csrf-value"})
	request.Header.Set(jwtauth.CSRFHeaderName, "csrf-value")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	setCookies := response.Header().Values("Set-Cookie")
	if len(setCookies) != 3 {
		t.Fatalf("Set-Cookie count = %d, want 3: %v", len(setCookies), setCookies)
	}
	joinedCookies := strings.Join(setCookies, "\n")
	if !strings.Contains(joinedCookies, "access_token=") || !strings.Contains(joinedCookies, "HttpOnly") || !strings.Contains(joinedCookies, "Secure") || !strings.Contains(joinedCookies, "SameSite=Lax") {
		t.Fatalf("access cookie attributes missing: %s", joinedCookies)
	}
	if !strings.Contains(joinedCookies, "refresh_token=") || !strings.Contains(joinedCookies, "Path=/api/v1/auth") {
		t.Fatalf("refresh cookie attributes missing: %s", joinedCookies)
	}
}

func TestLogoutHandlerRevokesAndClearsCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	refreshToken, err := jwtauth.GenerateRefreshToken("")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}
	store := &fakeRefreshSessionStore{}
	appConfig := config.Config{}
	appConfig.Auth.RefreshExpire = 3600
	appConfig.Auth.CookieSecure = true
	svcCtx := &svc.ServiceContext{Config: appConfig, RefreshSessions: store}
	router := gin.New()
	router.POST("/api/v1/auth/logout", middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), LogoutHandler(svcCtx))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: jwtauth.RefreshCookieName, Value: refreshToken.Raw})
	request.AddCookie(&http.Cookie{Name: jwtauth.CSRFCookieName, Value: "csrf-value"})
	request.Header.Set(jwtauth.CSRFHeaderName, "csrf-value")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !store.revoked {
		t.Fatalf("status = %d, revoked = %v, body = %s", response.Code, store.revoked, response.Body.String())
	}
	if !strings.Contains(strings.Join(response.Header().Values("Set-Cookie"), "\n"), "Max-Age=0") {
		t.Fatalf("logout did not clear cookies: %v", response.Header().Values("Set-Cookie"))
	}
}
