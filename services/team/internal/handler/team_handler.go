// Package handler wires Gin routes to TeamService.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/team/internal/service"
)

// TeamHandler ties HTTP to the service layer.
type TeamHandler struct {
	svc    *service.TeamService
	signer *jwtauth.Signer
}

// New constructs a TeamHandler.
func New(svc *service.TeamService, signer *jwtauth.Signer) *TeamHandler {
	return &TeamHandler{svc: svc, signer: signer}
}

// Register attaches routes to r.
func (h *TeamHandler) Register(r *gin.Engine) {
	// Inter-svc: no JWT (stage 1). Internal endpoint for asset-svc oversight check.
	r.GET("/managers/of/:uid", h.managersOf)

	authd := r.Group("/", jwtauth.Middleware(h.signer))
	authd.GET("/teams", h.list)
	authd.GET("/teams/:id", h.detail)

	mgr := authd.Group("/", jwtauth.RequireRole("manager"))
	mgr.POST("/teams", h.create)
	mgr.POST("/teams/:id/members", h.addMember)
	mgr.DELETE("/teams/:id/members/:uid", h.removeMember)
	mgr.POST("/teams/:id/managers", h.addManager)
	mgr.DELETE("/teams/:id/managers/:uid", h.removeManager)
}

func (h *TeamHandler) create(c *gin.Context) {
	var req createTeamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	caller := mustUID(c)
	t, err := h.svc.Create(c.Request.Context(), req.Name, caller)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (h *TeamHandler) list(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context(), 50, 0)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"teams": out})
}

func (h *TeamHandler) detail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	d, err := h.svc.Detail(c.Request.Context(), id, mustUID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *TeamHandler) addMember(c *gin.Context) {
	teamID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req addMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	target, err := uuid.Parse(req.UserID)
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	if err := h.svc.AddMember(c.Request.Context(), teamID, mustUID(c), target); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandler) removeMember(c *gin.Context) {
	teamID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	target, ok := parseUUIDParam(c, "uid")
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), teamID, mustUID(c), target); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandler) addManager(c *gin.Context) {
	teamID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req addManagerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	target, err := uuid.Parse(req.UserID)
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	if err := h.svc.AddManager(c.Request.Context(), teamID, mustUID(c), target); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandler) removeManager(c *gin.Context) {
	teamID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	target, ok := parseUUIDParam(c, "uid")
	if !ok {
		return
	}
	if err := h.svc.RemoveManager(c.Request.Context(), teamID, mustUID(c), target); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TeamHandler) managersOf(c *gin.Context) {
	uid, ok := parseUUIDParam(c, "uid")
	if !ok {
		return
	}
	mgrs, err := h.svc.ManagersOf(c.Request.Context(), uid)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": uid.String(), "managers": mgrs})
}

func mustUID(c *gin.Context) uuid.UUID {
	id, err := uuid.Parse(jwtauth.UID(c))
	if err != nil {
		// middleware guarantees a valid UID, but be defensive.
		return uuid.Nil
	}
	return id
}

func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return uuid.Nil, false
	}
	return id, true
}
