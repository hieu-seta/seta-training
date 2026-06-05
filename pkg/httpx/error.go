// Package httpx maps domain errors → HTTP status + JSON body.
// One central mapper means handlers stay dumb.
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Sentinel errors usable by service + repo layers.
var (
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("conflict")
	ErrForbidden   = errors.New("forbidden")
	ErrUnauthd     = errors.New("unauthenticated")
	ErrBadRequest  = errors.New("bad request")
	ErrUnavailable = errors.New("unavailable")
	ErrInternal    = errors.New("internal")
)

// Map returns the status + JSON body for an error. Defaults to 500.
func Map(err error) (int, gin.H) {
	switch {
	case err == nil:
		return http.StatusOK, gin.H{}
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, gin.H{"error": err.Error()}
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, gin.H{"error": err.Error()}
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, gin.H{"error": err.Error()}
	case errors.Is(err, ErrUnauthd):
		return http.StatusUnauthorized, gin.H{"error": err.Error()}
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, gin.H{"error": err.Error()}
	case errors.Is(err, ErrUnavailable):
		return http.StatusServiceUnavailable, gin.H{"error": err.Error()}
	default:
		return http.StatusInternalServerError, gin.H{"error": "internal"}
	}
}

// Abort writes status + body + aborts the gin chain.
func Abort(c *gin.Context, err error) {
	s, b := Map(err)
	c.AbortWithStatusJSON(s, b)
}
