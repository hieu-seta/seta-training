package authclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/authclient"
	"github.com/hieu-seta/seta-training/pkg/logger"
)

func stubServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/users/") || !strings.HasSuffix(r.URL.Path, "/exists") {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
	}))
}

func TestUserExists_200(t *testing.T) {
	s := stubServer(t, http.StatusOK)
	t.Cleanup(s.Close)
	c := authclient.New(s.URL, time.Second)
	ok, err := c.UserExists(context.Background(), uuid.New())
	if err != nil || !ok {
		t.Errorf("expected (true,nil), got (%v,%v)", ok, err)
	}
}

func TestUserExists_404(t *testing.T) {
	s := stubServer(t, http.StatusNotFound)
	t.Cleanup(s.Close)
	c := authclient.New(s.URL, time.Second)
	ok, err := c.UserExists(context.Background(), uuid.New())
	if err != nil || ok {
		t.Errorf("expected (false,nil), got (%v,%v)", ok, err)
	}
}

func TestUserExists_5xx_Unavailable(t *testing.T) {
	s := stubServer(t, http.StatusInternalServerError)
	t.Cleanup(s.Close)
	c := authclient.New(s.URL, time.Second)
	_, err := c.UserExists(context.Background(), uuid.New())
	if !authclient.IsUnavailable(err) {
		t.Errorf("expected unavailable, got %v", err)
	}
}

func TestUserExists_ForwardsRequestID(t *testing.T) {
	var got string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(logger.ReqIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	ctx := logger.ContextWithReqID(context.Background(), "trace-xyz")
	c := authclient.New(s.URL, time.Second)
	if _, err := c.UserExists(ctx, uuid.New()); err != nil {
		t.Fatalf("UserExists: %v", err)
	}
	if got != "trace-xyz" {
		t.Errorf("X-Request-Id = %q, want trace-xyz", got)
	}
}

func TestUserExists_NoRequestID_NoHeader(t *testing.T) {
	var got string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(logger.ReqIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	c := authclient.New(s.URL, time.Second)
	if _, err := c.UserExists(context.Background(), uuid.New()); err != nil {
		t.Fatalf("UserExists: %v", err)
	}
	if got != "" {
		t.Errorf("X-Request-Id should be empty, got %q", got)
	}
}

func TestUserExists_NetworkError_Unavailable(t *testing.T) {
	// Closed server → connection refused.
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := s.URL
	s.Close()
	c := authclient.New(url, 200*time.Millisecond)
	_, err := c.UserExists(context.Background(), uuid.New())
	if !authclient.IsUnavailable(err) {
		t.Errorf("expected unavailable, got %v", err)
	}
}
