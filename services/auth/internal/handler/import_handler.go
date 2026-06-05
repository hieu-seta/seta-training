package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/auth/internal/service"
)

// maxUploadBytes caps the multipart body in memory (5 MiB per spec).
const maxUploadBytes int64 = 5 << 20

// ImportHandler exposes POST /import-users (manager-only).
type ImportHandler struct {
	imp    *service.ImportService
	signer *jwtauth.Signer
}

// NewImportHandler ties svc + signer.
func NewImportHandler(imp *service.ImportService, signer *jwtauth.Signer) *ImportHandler {
	return &ImportHandler{imp: imp, signer: signer}
}

// Register attaches /import-users to r under JWT + manager-role guards.
func (h *ImportHandler) Register(r *gin.Engine) {
	g := r.Group("/", jwtauth.Middleware(h.signer), jwtauth.RequireRole("manager"))
	g.POST("/import-users", h.importUsers)
}

func (h *ImportHandler) importUsers(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	if err := c.Request.ParseMultipartForm(maxUploadBytes); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	f, _, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	defer func() { _ = f.Close() }()

	sum, err := h.imp.Run(c.Request.Context(), f)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	status := http.StatusOK
	if len(sum.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	c.JSON(status, sum)
}
