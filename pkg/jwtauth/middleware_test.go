package jwtauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
)

func setupRouter(s *jwtauth.Signer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/me", jwtauth.Middleware(s), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"uid": jwtauth.UID(c), "role": jwtauth.Role(c)})
	})
	r.GET("/admin", jwtauth.Middleware(s), jwtauth.RequireRole("manager"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestMiddleware_NoHeader_401(t *testing.T) {
	r := setupRouter(jwtauth.NewSigner(testSecret))
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me", http.NoBody)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestMiddleware_BadScheme_401(t *testing.T) {
	r := setupRouter(jwtauth.NewSigner(testSecret))
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me", http.NoBody)
	req.Header.Set("Authorization", "Basic abcd")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestMiddleware_ValidToken_PassesUIDRole(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	tok, _ := s.MintAccess("u-7", "member", time.Minute)
	r := setupRouter(s)
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	if want := `"uid":"u-7"`; !contains(w.Body.String(), want) {
		t.Errorf("body missing %s: %s", want, w.Body.String())
	}
}

func TestRequireRole_RejectsWrongRole(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	tok, _ := s.MintAccess("u", "member", time.Minute)
	r := setupRouter(s)
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

func TestRequireRole_AcceptsManager(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	tok, _ := s.MintAccess("u", "manager", time.Minute)
	r := setupRouter(s)
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
