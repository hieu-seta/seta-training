package teamclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/logger"
	"github.com/hieu-seta/seta-training/pkg/teamclient"
)

func TestManagersOf_Happy(t *testing.T) {
	mgrA := uuid.New()
	mgrB := uuid.New()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id":  uuid.NewString(),
			"managers": []string{mgrA.String(), mgrB.String()},
		})
	}))
	t.Cleanup(s.Close)
	c := teamclient.New(s.URL, time.Second)
	mgrs, err := c.ManagersOf(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(mgrs) != 2 {
		t.Errorf("want 2, got %d", len(mgrs))
	}
}

func TestManagersOf_5xx_Unavailable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(s.Close)
	c := teamclient.New(s.URL, time.Second)
	_, err := c.ManagersOf(context.Background(), uuid.New())
	if !errors.Is(err, httpx.ErrUnavailable) {
		t.Errorf("want unavailable, got %v", err)
	}
}

func TestManagersOf_ForwardsRequestID(t *testing.T) {
	var got string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(logger.ReqIDHeader)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id":  uuid.NewString(),
			"managers": []string{},
		})
	}))
	t.Cleanup(s.Close)
	ctx := logger.ContextWithReqID(context.Background(), "trace-abc")
	c := teamclient.New(s.URL, time.Second)
	if _, err := c.ManagersOf(ctx, uuid.New()); err != nil {
		t.Fatalf("ManagersOf: %v", err)
	}
	if got != "trace-abc" {
		t.Errorf("X-Request-Id = %q, want trace-abc", got)
	}
}

func TestManagersOf_NetworkError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := s.URL
	s.Close()
	c := teamclient.New(url, 100*time.Millisecond)
	_, err := c.ManagersOf(context.Background(), uuid.New())
	if !errors.Is(err, httpx.ErrUnavailable) {
		t.Errorf("want unavailable, got %v", err)
	}
}
