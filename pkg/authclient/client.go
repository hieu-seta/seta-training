// Package authclient is a thin HTTP client to auth-svc.
// One method today (UserExists); grow as needed.
package authclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/logger"
)

// AuthClient = port. Implementations: HTTP, in-memory (tests).
type AuthClient interface {
	UserExists(ctx context.Context, uid uuid.UUID) (bool, error)
}

// HTTPClient hits auth-svc HTTP endpoints.
type HTTPClient struct {
	base string
	c    *http.Client
}

// New builds an HTTPClient. timeout applies per-request.
func New(baseURL string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HTTPClient{
		base: baseURL,
		c:    &http.Client{Timeout: timeout},
	}
}

// UserExists returns (true,nil) on 200, (false,nil) on 404, (false,err) otherwise.
// Errors propagate as httpx.ErrUnavailable so handlers can map to 503.
func (h *HTTPClient) UserExists(ctx context.Context, uid uuid.UUID) (bool, error) {
	url := fmt.Sprintf("%s/users/%s/exists", h.base, uid.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("%w: build req: %w", httpx.ErrUnavailable, err)
	}
	if id := logger.ReqIDFromContext(ctx); id != "" {
		req.Header.Set(logger.ReqIDHeader, id)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: auth-svc: %w", httpx.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("%w: auth-svc unexpected status %d", httpx.ErrUnavailable, resp.StatusCode)
	}
}

// IsUnavailable reports whether err originated from a downstream outage.
func IsUnavailable(err error) bool {
	return errors.Is(err, httpx.ErrUnavailable)
}
