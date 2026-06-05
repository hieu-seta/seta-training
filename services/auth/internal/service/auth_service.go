package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/auth/internal/model"
	"github.com/hieu-seta/seta-training/services/auth/internal/repo"
	"golang.org/x/crypto/bcrypt"
)

// Config = ttls + bcrypt cost.
type Config struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	BcryptCost int
}

// TokenPair returned to clients on login/refresh.
type TokenPair struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}

// AuthService = orchestrates users + tokens.
type AuthService struct {
	users    repo.UserRepo
	refresh  repo.RefreshRepo
	signer   *jwtauth.Signer
	cfg      Config
	newUUID  func() uuid.UUID
	nowClock func() time.Time
}

// New constructs an AuthService.
func New(users repo.UserRepo, refresh repo.RefreshRepo, signer *jwtauth.Signer, cfg Config) *AuthService {
	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 7 * 24 * time.Hour
	}
	if cfg.BcryptCost == 0 {
		cfg.BcryptCost = bcrypt.DefaultCost
	}
	return &AuthService{
		users:    users,
		refresh:  refresh,
		signer:   signer,
		cfg:      cfg,
		newUUID:  uuid.New,
		nowClock: time.Now,
	}
}

// Register creates a new user. Email is canonicalized (lowercased + trimmed).
func (s *AuthService) Register(ctx context.Context, username, email, password, role string) (*model.User, error) {
	username = strings.TrimSpace(username)
	email = canonEmail(email)
	if username == "" || email == "" || password == "" {
		return nil, fmt.Errorf("%w: missing field", httpx.ErrBadRequest)
	}
	if !model.IsValidRole(role) {
		return nil, fmt.Errorf("%w: invalid role", httpx.ErrBadRequest)
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("%w: password too short (min 8)", httpx.ErrBadRequest)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cfg.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash: %w", err)
	}
	u := &model.User{
		ID:           s.newUUID(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Login verifies pw + mints a token pair (new family).
func (s *AuthService) Login(ctx context.Context, email, password string) (*model.User, *TokenPair, error) {
	u, err := s.users.ByEmail(ctx, canonEmail(email))
	if err != nil {
		if errors.Is(err, httpx.ErrNotFound) {
			return nil, nil, httpx.ErrUnauthd
		}
		return nil, nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, nil, httpx.ErrUnauthd
	}
	pair, err := s.issuePair(ctx, u, s.newUUID().String())
	if err != nil {
		return nil, nil, err
	}
	return u, pair, nil
}

// Refresh rotates a refresh token. Reuse of an old jti → wipe family + 401.
func (s *AuthService) Refresh(ctx context.Context, refresh string) (*TokenPair, error) {
	claims, err := s.signer.Parse(refresh)
	if err != nil {
		return nil, httpx.ErrUnauthd
	}
	uid, fam, err := s.refresh.Lookup(ctx, claims.ID)
	if err != nil {
		// Token parses but not in store → already used or wiped → assume theft.
		if errors.Is(err, httpx.ErrUnauthd) && claims.Family != "" {
			_ = s.refresh.DeleteFamily(ctx, claims.Family)
		}
		return nil, httpx.ErrUnauthd
	}
	if uid != claims.UID || fam != claims.Family {
		_ = s.refresh.DeleteFamily(ctx, claims.Family)
		return nil, httpx.ErrUnauthd
	}
	// Old jti → delete, mint new w/ same family.
	if err := s.refresh.Delete(ctx, claims.ID); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(uid)
	if err != nil {
		return nil, httpx.ErrInternal
	}
	u, err := s.users.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.issuePair(ctx, u, fam)
}

// Logout deletes a single refresh jti. Idempotent.
func (s *AuthService) Logout(ctx context.Context, refresh string) error {
	claims, err := s.signer.Parse(refresh)
	if err != nil {
		return nil // already invalid → nothing to do
	}
	return s.refresh.Delete(ctx, claims.ID)
}

// ListUsers — for stage 1, any authenticated caller can list.
func (s *AuthService) ListUsers(ctx context.Context, limit, offset int) ([]model.User, error) {
	return s.users.List(ctx, limit, offset)
}

// UserExists — used by team-svc + asset-svc.
func (s *AuthService) UserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.users.Exists(ctx, id)
}

func (s *AuthService) issuePair(ctx context.Context, u *model.User, family string) (*TokenPair, error) {
	access, err := s.signer.MintAccess(u.ID.String(), u.Role, s.cfg.AccessTTL)
	if err != nil {
		return nil, err
	}
	jti := s.newUUID().String()
	refresh, err := s.signer.MintRefresh(u.ID.String(), u.Role, jti, family, s.cfg.RefreshTTL)
	if err != nil {
		return nil, err
	}
	if err := s.refresh.Store(ctx, jti, u.ID.String(), family, s.cfg.RefreshTTL); err != nil {
		return nil, err
	}
	return &TokenPair{Access: access, Refresh: refresh}, nil
}

func canonEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
