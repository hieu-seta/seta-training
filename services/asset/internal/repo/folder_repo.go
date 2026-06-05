// Package repo holds GORM-backed persistence for asset-svc.
package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/asset/internal/model"
	"gorm.io/gorm"
)

// FolderRepo = persistence port for folders + folder_shares.
type FolderRepo interface {
	Create(ctx context.Context, f *model.Folder) error
	ByID(ctx context.Context, id uuid.UUID) (*model.Folder, error)
	Update(ctx context.Context, f *model.Folder) error
	Delete(ctx context.Context, id uuid.UUID) error

	// ListVisible returns folders owned by uid OR shared w/ uid OR owned by team members uid manages.
	ListVisible(ctx context.Context, uid uuid.UUID, managedOwners []uuid.UUID, limit, offset int) ([]model.Folder, error)

	Share(ctx context.Context, s *model.FolderShare) error
	Unshare(ctx context.Context, folderID, uid uuid.UUID) error
	ShareAccess(ctx context.Context, folderID, uid uuid.UUID) (string, error)
}

type gormFolderRepo struct{ db *gorm.DB }

// NewFolderRepo builds a GORM-backed FolderRepo.
func NewFolderRepo(db *gorm.DB) FolderRepo { return &gormFolderRepo{db: db} }

func (r *gormFolderRepo) Create(ctx context.Context, f *model.Folder) error {
	return r.db.WithContext(ctx).Create(f).Error
}

func (r *gormFolderRepo) ByID(ctx context.Context, id uuid.UUID) (*model.Folder, error) {
	var f model.Folder
	err := r.db.WithContext(ctx).First(&f, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *gormFolderRepo) Update(ctx context.Context, f *model.Folder) error {
	res := r.db.WithContext(ctx).Save(f)
	return res.Error
}

func (r *gormFolderRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Folder{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (r *gormFolderRepo) ListVisible(ctx context.Context, uid uuid.UUID, managed []uuid.UUID, limit, offset int) ([]model.Folder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := r.db.WithContext(ctx).
		Distinct("folders.*").
		Table("folders").
		Joins("LEFT JOIN folder_shares fs ON fs.folder_id = folders.id AND fs.user_id = ?", uid)
	if len(managed) > 0 {
		q = q.Where("folders.owner_id = ? OR fs.user_id = ? OR folders.owner_id IN ?", uid, uid, managed)
	} else {
		q = q.Where("folders.owner_id = ? OR fs.user_id = ?", uid, uid)
	}
	var out []model.Folder
	err := q.Order("folders.created_at DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, err
}

func (r *gormFolderRepo) Share(ctx context.Context, s *model.FolderShare) error {
	// Upsert: re-sharing same (folder,user) updates access.
	err := r.db.WithContext(ctx).Save(s).Error
	if err != nil && strings.Contains(err.Error(), "violates check") {
		return httpx.ErrBadRequest
	}
	return err
}

func (r *gormFolderRepo) Unshare(ctx context.Context, folderID, uid uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("folder_id = ? AND user_id = ?", folderID, uid).Delete(&model.FolderShare{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (r *gormFolderRepo) ShareAccess(ctx context.Context, folderID, uid uuid.UUID) (string, error) {
	var s model.FolderShare
	err := r.db.WithContext(ctx).Where("folder_id = ? AND user_id = ?", folderID, uid).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s.Access, nil
}
