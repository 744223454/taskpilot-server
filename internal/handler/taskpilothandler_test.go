package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/gin-gonic/gin"
)

func TestReadinessHandlerRequiresAllDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", ReadinessHandler(&svc.ServiceContext{}))

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
