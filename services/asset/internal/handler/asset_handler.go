// Package handler wires Gin routes to AssetService.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/asset/internal/service"
)

// AssetHandler ties HTTP to the service layer.
type AssetHandler struct {
	svc    *service.AssetService
	signer *jwtauth.Signer
}

// New constructs an AssetHandler.
func New(svc *service.AssetService, signer *jwtauth.Signer) *AssetHandler {
	return &AssetHandler{svc: svc, signer: signer}
}

// Register attaches routes.
func (h *AssetHandler) Register(r *gin.Engine) {
	authd := r.Group("/", jwtauth.Middleware(h.signer))

	authd.POST("/folders", h.createFolder)
	authd.GET("/folders", h.listFolders)
	authd.GET("/folders/:id", h.getFolder)
	authd.PUT("/folders/:id", h.updateFolder)
	authd.DELETE("/folders/:id", h.deleteFolder)
	authd.POST("/folders/:id/share", h.shareFolder)
	authd.DELETE("/folders/:id/share/:uid", h.unshareFolder)

	authd.POST("/folders/:id/notes", h.createNote)
	authd.GET("/notes/:id", h.getNote)
	authd.PUT("/notes/:id", h.updateNote)
	authd.DELETE("/notes/:id", h.deleteNote)
	authd.POST("/notes/:id/share", h.shareNote)
	authd.DELETE("/notes/:id/share/:uid", h.unshareNote)
}

func (h *AssetHandler) createFolder(c *gin.Context) {
	var req folderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	f, err := h.svc.CreateFolder(c.Request.Context(), req.Name, callerUID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, f)
}

func (h *AssetHandler) listFolders(c *gin.Context) {
	out, err := h.svc.ListFolders(c.Request.Context(), callerUID(c), 50, 0)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"folders": out})
}

func (h *AssetHandler) getFolder(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	f, err := h.svc.GetFolder(c.Request.Context(), id, callerUID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *AssetHandler) updateFolder(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	var req folderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	f, err := h.svc.UpdateFolder(c.Request.Context(), id, callerUID(c), req.Name)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *AssetHandler) deleteFolder(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteFolder(c.Request.Context(), id, callerUID(c)); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AssetHandler) shareFolder(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	target, access, ok := parseShareBody(c)
	if !ok {
		return
	}
	if err := h.svc.ShareFolder(c.Request.Context(), id, callerUID(c), target, access); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AssetHandler) unshareFolder(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	uid, ok := parseUUID(c, "uid")
	if !ok {
		return
	}
	if err := h.svc.UnshareFolder(c.Request.Context(), id, callerUID(c), uid); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AssetHandler) createNote(c *gin.Context) {
	folderID, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	var req noteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	n, err := h.svc.CreateNote(c.Request.Context(), folderID, callerUID(c), req.Title, req.Body)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, n)
}

func (h *AssetHandler) getNote(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	n, err := h.svc.GetNote(c.Request.Context(), id, callerUID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, n)
}

func (h *AssetHandler) updateNote(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	var req noteUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	n, err := h.svc.UpdateNote(c.Request.Context(), id, callerUID(c), req.Title, req.Body)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, n)
}

func (h *AssetHandler) deleteNote(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteNote(c.Request.Context(), id, callerUID(c)); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AssetHandler) shareNote(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	target, access, ok := parseShareBody(c)
	if !ok {
		return
	}
	if err := h.svc.ShareNote(c.Request.Context(), id, callerUID(c), target, access); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AssetHandler) unshareNote(c *gin.Context) {
	id, ok := parseUUID(c, "id")
	if !ok {
		return
	}
	uid, ok := parseUUID(c, "uid")
	if !ok {
		return
	}
	if err := h.svc.UnshareNote(c.Request.Context(), id, callerUID(c), uid); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func callerUID(c *gin.Context) uuid.UUID {
	id, err := uuid.Parse(jwtauth.UID(c))
	if err != nil {
		return uuid.Nil
	}
	return id
}

func parseUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return uuid.Nil, false
	}
	return id, true
}

func parseShareBody(c *gin.Context) (uuid.UUID, string, bool) {
	var req shareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return uuid.Nil, "", false
	}
	id, err := uuid.Parse(req.UserID)
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return uuid.Nil, "", false
	}
	return id, req.Access, true
}
