package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/gin-gonic/gin"
)

func TestProtectedRoutesRequireAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, &svc.ServiceContext{
		JWT: jwtauth.NewManager("test-secret", 3600),
	})

	for _, path := range []string{"/api/v1/users/me", "/api/v1/documents"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestCreateTextDocumentRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtManager := jwtauth.NewManager("test-secret", 3600)
	router := gin.New()
	RegisterRoutes(router, &svc.ServiceContext{JWT: jwtManager})

	token, err := jwtManager.GenerateToken(jwtauth.Claims{UserID: 1})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	body := bytes.NewBufferString(`{"title":"large","text":"` + strings.Repeat("a", 300000) + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/documents/text", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST /api/v1/documents/text status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCreateTextDocumentRejectsTooManyCharacters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtManager := jwtauth.NewManager("test-secret", 3600)
	router := gin.New()
	RegisterRoutes(router, &svc.ServiceContext{JWT: jwtManager})

	token, err := jwtManager.GenerateToken(jwtauth.Claims{UserID: 1})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	body := bytes.NewBufferString(`{"title":"large","text":"` + strings.Repeat("a", 50001) + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/documents/text", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/documents/text status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
