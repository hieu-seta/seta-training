package httpx_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/hieu-seta/seta-training/pkg/httpx"
)

func TestMap_SentinelStatuses(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, http.StatusOK},
		{httpx.ErrNotFound, http.StatusNotFound},
		{httpx.ErrConflict, http.StatusConflict},
		{httpx.ErrForbidden, http.StatusForbidden},
		{httpx.ErrUnauthd, http.StatusUnauthorized},
		{httpx.ErrBadRequest, http.StatusBadRequest},
		{httpx.ErrUnavailable, http.StatusServiceUnavailable},
		{errors.New("random"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%v", tc.err), func(t *testing.T) {
			got, _ := httpx.Map(tc.err)
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

// Wrapped sentinels must still match.
func TestMap_WrappedNotFound(t *testing.T) {
	err := fmt.Errorf("user lookup: %w", httpx.ErrNotFound)
	got, _ := httpx.Map(err)
	if got != http.StatusNotFound {
		t.Errorf("wrapped: got %d want %d", got, http.StatusNotFound)
	}
}
