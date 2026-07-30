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

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/users/me"},
		{method: http.MethodGet, path: "/api/v1/documents"},
		{method: http.MethodGet, path: "/api/v1/parse-jobs/1/result"},
		{method: http.MethodGet, path: "/api/v1/parse-results/1"},
		{method: http.MethodPost, path: "/api/v1/projects"},
		{method: http.MethodGet, path: "/api/v1/projects"},
		{method: http.MethodPut, path: "/api/v1/projects/1"},
		{method: http.MethodPost, path: "/api/v1/projects/1/tasks"},
		{method: http.MethodPatch, path: "/api/v1/tasks/1/status"},
		{method: http.MethodPost, path: "/api/v1/tasks/reorder"},
		{method: http.MethodGet, path: "/api/v1/history/projects"},
		{method: http.MethodGet, path: "/api/v1/history/parse-results"},
	} {
		request := httptest.NewRequest(testCase.method, testCase.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want %d", testCase.method, testCase.path, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestCreateProjectRejectsWhitespaceName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtManager := jwtauth.NewManager("test-secret", 3600)
	router := gin.New()
	RegisterRoutes(router, &svc.ServiceContext{JWT: jwtManager})

	token, err := jwtManager.GenerateToken(jwtauth.Claims{UserID: 1})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(`{"parse_result_id":1,"name":"   "}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/projects status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestCreateTaskRejectsClientControlledStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtManager := jwtauth.NewManager("test-secret", 3600)
	router := gin.New()
	RegisterRoutes(router, &svc.ServiceContext{JWT: jwtManager})

	token, err := jwtManager.GenerateToken(jwtauth.Claims{UserID: 1})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/1/tasks", bytes.NewBufferString(`{"title":"Task","priority":"medium","status":"done"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/projects/1/tasks status = %d, want %d", response.Code, http.StatusBadRequest)
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
