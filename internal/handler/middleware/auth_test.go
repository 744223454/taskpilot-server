package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/gin-gonic/gin"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    string
		wantErr bool
	}{
		{name: "valid", header: "Bearer token", want: "token"},
		{name: "case insensitive", header: "bearer token", want: "token"},
		{name: "extra spaces", header: "  Bearer   token  ", want: "token"},
		{name: "missing token", header: "Bearer", wantErr: true},
		{name: "wrong scheme", header: "Basic token", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := bearerToken(test.header)
			if test.wantErr {
				if err == nil {
					t.Fatal("bearerToken() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("bearerToken() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("bearerToken() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPrincipalFrom(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := Principal{UserID: 42, Email: "test@example.com", Nickname: "tester"}
	context.Set(principalKey, want)

	got, ok := PrincipalFrom(context)
	if !ok {
		t.Fatal("PrincipalFrom() expected principal")
	}
	if got != want {
		t.Fatalf("PrincipalFrom() = %#v, want %#v", got, want)
	}
}

func TestRequireAuthPrefersBearerOverCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := jwtauth.NewManager("test-secret", 3600)
	bearer, err := manager.GenerateToken(jwtauth.Claims{UserID: 42})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	router := gin.New()
	router.Use(RequireAuth(&svc.ServiceContext{JWT: manager}))
	router.GET("/protected", func(c *gin.Context) {
		principal, ok := PrincipalFrom(c)
		if !ok || principal.UserID != 42 {
			t.Fatalf("principal = %#v, ok = %v", principal, ok)
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.AddCookie(&http.Cookie{Name: jwtauth.AccessCookieName, Value: "expired-cookie"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestRequireCSRFForCookieAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := jwtauth.NewManager("test-secret", 3600)
	token, err := manager.GenerateToken(jwtauth.Claims{UserID: 42})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	router := gin.New()
	router.Use(RequireAuth(&svc.ServiceContext{JWT: manager}), RequireCSRFForCookieAuth())
	router.POST("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	withoutCSRF := httptest.NewRequest(http.MethodPost, "/protected", nil)
	withoutCSRF.AddCookie(&http.Cookie{Name: jwtauth.AccessCookieName, Value: token})
	withoutCSRF.AddCookie(&http.Cookie{Name: jwtauth.CSRFCookieName, Value: "csrf-value"})
	withoutCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(withoutCSRFResponse, withoutCSRF)
	if withoutCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d, want %d", withoutCSRFResponse.Code, http.StatusForbidden)
	}

	withCSRF := httptest.NewRequest(http.MethodPost, "/protected", nil)
	withCSRF.AddCookie(&http.Cookie{Name: jwtauth.AccessCookieName, Value: token})
	withCSRF.AddCookie(&http.Cookie{Name: jwtauth.CSRFCookieName, Value: "csrf-value"})
	withCSRF.Header.Set(jwtauth.CSRFHeaderName, "csrf-value")
	withCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(withCSRFResponse, withCSRF)
	if withCSRFResponse.Code != http.StatusNoContent {
		t.Fatalf("valid csrf status = %d, want %d", withCSRFResponse.Code, http.StatusNoContent)
	}
}
