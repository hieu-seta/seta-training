// Package teamclient is a thin HTTP client to team-svc.
// Used by asset-svc to evaluate the "manager oversight" RBAC rule.
package teamclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/logger"
)

// TeamClient is the port.
type TeamClient interface {
	ManagersOf(ctx context.Context, uid uuid.UUID) ([]uuid.UUID, error)
}

// HTTPClient hits team-svc /managers/of/:uid.
type HTTPClient struct {
	base string
	c    *http.Client
}

// New builds an HTTPClient w/ per-request timeout.
func New(baseURL string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HTTPClient{base: baseURL, c: &http.Client{Timeout: timeout}}
}

type managersResp struct {
	UserID   string      `json:"user_id"`
	Managers []uuid.UUID `json:"managers"`
}

// ManagersOf returns the managers across all teams uid is a member of.
// Empty list (no error) when uid is in no teams.
func (h *HTTPClient) ManagersOf(ctx context.Context, uid uuid.UUID) ([]uuid.UUID, error) {
	url := fmt.Sprintf("%s/managers/of/%s", h.base, uid.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: build req: %w", httpx.ErrUnavailable, err)
	}
	if id := logger.ReqIDFromContext(ctx); id != "" {
		req.Header.Set(logger.ReqIDHeader, id)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: team-svc: %w", httpx.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: team-svc status %d", httpx.ErrUnavailable, resp.StatusCode)
	}
	var body managersResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", httpx.ErrUnavailable, err)
	}
	return body.Managers, nil
}
