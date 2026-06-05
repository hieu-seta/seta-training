package logger

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReqIDHeader is the HTTP header carrying the request id across services.
const ReqIDHeader = "X-Request-Id"

// CtxReqID is the gin context key for the request id.
const CtxReqID = "req_id"

// reqIDKey is the typed key used in standard context.Context to avoid collisions.
type reqIDKey struct{}

// ContextWithReqID returns a derived context carrying id. Empty id is a no-op.
func ContextWithReqID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, reqIDKey{}, id)
}

// ReqIDFromContext extracts the request id from ctx, or returns "".
func ReqIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(reqIDKey{}).(string)
	return v
}

// ReqID middleware reads X-Request-Id (or mints a uuid), stashes in ctx, echoes in response.
// The id lands in both the gin context (for GetReqID) and the request context (for ReqIDFromContext).
func ReqID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(ReqIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(CtxReqID, id)
		c.Writer.Header().Set(ReqIDHeader, id)
		c.Request = c.Request.WithContext(ContextWithReqID(c.Request.Context(), id))
		c.Next()
	}
}

// GetReqID pulls the req_id from a gin context (empty if absent).
func GetReqID(c *gin.Context) string {
	v, _ := c.Get(CtxReqID)
	s, _ := v.(string)
	return s
}
