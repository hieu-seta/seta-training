package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/auth/internal/model"
	"gorm.io/gorm"
)

// UserRepo is the persistence port — service depends on this.
type UserRepo interface {
	Create(ctx context.Context, u *model.User) error
	ByEmail(ctx context.Context, email string) (*model.User, error)
	ByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	List(ctx context.Context, limit, offset int) ([]model.User, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type gormUserRepo struct{ db *gorm.DB }

// NewUserRepo constructs the GORM-backed UserRepo.
func NewUserRepo(db *gorm.DB) UserRepo { return &gormUserRepo{db: db} }

func (r *gormUserRepo) Create(ctx context.Context, u *model.User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		if isUniqueViolation(err) {
			return httpx.ErrConflict
		}
		return err
	}
	return nil
}

func (r *gormUserRepo) ByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("email = ?", strings.ToLower(email)).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormUserRepo) ByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormUserRepo) List(ctx context.Context, limit, offset int) ([]model.User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var out []model.User
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, err
}

func (r *gormUserRepo) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("users").Where("id = ?", id).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// isUniqueViolation matches PG SQLSTATE 23505 without depending on pgconn (keeps deps lean).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}
