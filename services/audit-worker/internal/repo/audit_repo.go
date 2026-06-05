// Package repo persists audit events.
package repo

import (
	"context"

	"github.com/hieu-seta/seta-training/services/audit-worker/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AuditRepo is the persistence port.
type AuditRepo interface {
	// Insert returns (inserted bool, err). inserted=false when idem_key already existed.
	Insert(ctx context.Context, e *model.Event) (bool, error)
}

type gormRepo struct{ db *gorm.DB }

// New builds a GORM-backed AuditRepo.
func New(db *gorm.DB) AuditRepo { return &gormRepo{db: db} }

func (r *gormRepo) Insert(ctx context.Context, e *model.Event) (bool, error) {
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idem_key"}},
		DoNothing: true,
	}).Create(e)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
