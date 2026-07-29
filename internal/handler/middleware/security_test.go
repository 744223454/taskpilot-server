package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSecureRedirectsInsecureRequest(t *testing.T) {
	router := gin.New()
	router.Use(Secure(true))
	router.GET("/private", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "http://api.example.com/private?source=test", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusPermanentRedirect, response.Code)
	}
	if location := response.Header().Get("Location"); location != "https://api.example.com/private?source=test" {
		t.Fatalf("unexpected redirect location %q", location)
	}
}

func TestSecureAcceptsForwardedHTTPSAndSetsHSTS(t *testing.T) {
	router := gin.New()
	router.Use(Secure(true))
	router.GET("/private", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "http://api.example.com/private", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}
	if hsts := response.Header().Get("Strict-Transport-Security"); hsts != strictTransportSecurity {
		t.Fatalf("unexpected HSTS header %q", hsts)
	}
}

func TestNoCacheSetsSensitiveResponseHeaders(t *testing.T) {
	router := gin.New()
	router.Use(NoCache())
	router.GET("/private", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store, no-cache, must-revalidate, private" {
		t.Fatalf("unexpected Cache-Control header %q", cacheControl)
	}
	if pragma := response.Header().Get("Pragma"); pragma != "no-cache" {
		t.Fatalf("unexpected Pragma header %q", pragma)
	}
	if expires := response.Header().Get("Expires"); expires != "0" {
		t.Fatalf("unexpected Expires header %q", expires)
	}
}

func TestTimeoutAddsRequestDeadline(t *testing.T) {
	router := gin.New()
	router.Use(Timeout(time.Second))
	deadlineFound := false
	router.GET("/private", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		deadlineFound = ok && time.Until(deadline) > 0 && time.Until(deadline) <= time.Second
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}
	if !deadlineFound {
		t.Fatal("expected request context to include a timeout deadline")
	}
}
