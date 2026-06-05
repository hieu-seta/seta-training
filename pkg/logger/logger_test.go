package logger_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hieu-seta/seta-training/pkg/logger"
)

func TestNew_WritesJSON(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New("test-svc", "info", &buf)
	l.Info().Str("k", "v").Msg("hi")
	s := buf.String()
	if !strings.Contains(s, `"svc":"test-svc"`) || !strings.Contains(s, `"k":"v"`) || !strings.Contains(s, `"message":"hi"`) {
		t.Errorf("missing fields: %s", s)
	}
}

func TestGinMiddleware_LogsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	l := logger.New("test-svc", "info", &buf)
	r := gin.New()
	r.Use(logger.GinMiddleware(l))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody))
	if !strings.Contains(buf.String(), `"path":"/x"`) {
		t.Errorf("expected path in log: %s", buf.String())
	}
}

func TestNew_InvalidLevelDefaultsInfo(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New("s", "garbage", &buf)
	l.Debug().Msg("hidden")
	l.Info().Msg("visible")
	if strings.Contains(buf.String(), "hidden") {
		t.Errorf("debug should be filtered: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "visible") {
		t.Errorf("info missing: %s", buf.String())
	}
}
