package logger_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hieu-seta/seta-training/pkg/logger"
)

func setupReqIDRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(logger.ReqID())
	r.GET("/echo", func(c *gin.Context) {
		c.String(http.StatusOK, logger.GetReqID(c))
	})
	return r
}

func TestReqID_GeneratesWhenMissing(t *testing.T) {
	r := setupReqIDRouter()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/echo", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Errorf("expected generated id in body")
	}
	if w.Header().Get(logger.ReqIDHeader) == "" {
		t.Errorf("expected response header set")
	}
}

func TestReqID_PreservesIncoming(t *testing.T) {
	r := setupReqIDRouter()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/echo", http.NoBody)
	req.Header.Set(logger.ReqIDHeader, "abc-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "abc-123" {
		t.Errorf("expected abc-123 echoed, got %q", w.Body.String())
	}
	if w.Header().Get(logger.ReqIDHeader) != "abc-123" {
		t.Errorf("expected response header preserved")
	}
}

func TestReqID_InjectsIntoRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(logger.ReqID())
	var seen string
	r.GET("/probe", func(c *gin.Context) {
		seen = logger.ReqIDFromContext(c.Request.Context())
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", http.NoBody)
	req.Header.Set(logger.ReqIDHeader, "ctx-id")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "ctx-id" {
		t.Errorf("ReqIDFromContext = %q, want ctx-id", seen)
	}
}

func TestContextWithReqID_EmptyIsNoop(t *testing.T) {
	ctx := logger.ContextWithReqID(context.Background(), "")
	if got := logger.ReqIDFromContext(ctx); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestReqIDFromContext_Nil(t *testing.T) {
	if got := logger.ReqIDFromContext(nil); got != "" {
		t.Errorf("nil ctx should return empty, got %q", got)
	}
}
