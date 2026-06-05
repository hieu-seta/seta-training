package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/auth/internal/service"
)

// AuthHandler wires Gin routes to AuthService.
type AuthHandler struct {
	svc    *service.AuthService
	signer *jwtauth.Signer
}

// New constructs an AuthHandler. svc + signer are injected.
func New(svc *service.AuthService, signer *jwtauth.Signer) *AuthHandler {
	return &AuthHandler{svc: svc, signer: signer}
}

// Register attaches routes to an existing *gin.Engine.
func (h *AuthHandler) Register(r *gin.Engine) {
	r.POST("/users", h.register)
	r.POST("/login", h.login)
	r.POST("/refresh", h.refresh)
	r.POST("/logout", h.logout)

	// Inter-svc check: no JWT (stage 1; phase-09 may add internal token).
	r.GET("/users/:id/exists", h.exists)

	// Protected
	auth := r.Group("/", jwtauth.Middleware(h.signer))
	auth.GET("/users", h.list)
	auth.GET("/me", h.me)
}

func (h *AuthHandler) register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	u, err := h.svc.Register(c.Request.Context(), req.Username, req.Email, req.Password, req.Role)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, u)
}

func (h *AuthHandler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	_, pair, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, tokenResp{Access: pair.Access, Refresh: pair.Refresh})
}

func (h *AuthHandler) refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.Refresh)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, tokenResp{Access: pair.Access, Refresh: pair.Refresh})
}

func (h *AuthHandler) logout(c *gin.Context) {
	var req logoutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.Refresh); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) list(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context(), 50, 0)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (h *AuthHandler) exists(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Abort(c, errors.Join(httpx.ErrBadRequest, err))
		return
	}
	ok, err := h.svc.UserExists(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if !ok {
		httpx.Abort(c, httpx.ErrNotFound)
		return
	}
	c.JSON(http.StatusOK, existsResp{Exists: true, ID: id.String()})
}

func (h *AuthHandler) me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"uid": jwtauth.UID(c), "role": jwtauth.Role(c)})
}
