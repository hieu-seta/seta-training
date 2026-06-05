package repo

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/team/internal/model"
	"gorm.io/gorm"
)

// TeamRepo is the persistence port for teams + members + managers.
type TeamRepo interface {
	Create(ctx context.Context, t *model.Team, mainMgrUID uuid.UUID) error
	ByID(ctx context.Context, id uuid.UUID) (*model.Team, error)
	List(ctx context.Context, limit, offset int) ([]model.Team, error)
	Detail(ctx context.Context, id uuid.UUID) (*model.TeamDetail, error)

	IsManager(ctx context.Context, teamID, uid uuid.UUID) (bool, error)
	IsMainManager(ctx context.Context, teamID, uid uuid.UUID) (bool, error)
	IsMember(ctx context.Context, teamID, uid uuid.UUID) (bool, error)

	AddMember(ctx context.Context, teamID, uid uuid.UUID) error
	RemoveMember(ctx context.Context, teamID, uid uuid.UUID) error
	AddManager(ctx context.Context, teamID, uid uuid.UUID) error
	RemoveManager(ctx context.Context, teamID, uid uuid.UUID) error
	CountMainManagers(ctx context.Context, teamID uuid.UUID) (int64, error)
	// ManagersOf returns the union of managers across all teams `uid` is a member of.
	// Used by asset-svc for the "manager oversight" RBAC rule.
	ManagersOf(ctx context.Context, uid uuid.UUID) ([]uuid.UUID, error)
}

type gormTeamRepo struct{ db *gorm.DB }

// New builds a GORM-backed TeamRepo.
func New(db *gorm.DB) TeamRepo { return &gormTeamRepo{db: db} }

func (r *gormTeamRepo) Create(ctx context.Context, t *model.Team, mainMgrUID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			if isUnique(err) {
				return httpx.ErrConflict
			}
			return err
		}
		mgr := &model.TeamManager{TeamID: t.ID, UserID: mainMgrUID, IsMain: true}
		return tx.Create(mgr).Error
	})
}

func (r *gormTeamRepo) ByID(ctx context.Context, id uuid.UUID) (*model.Team, error) {
	var t model.Team
	err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *gormTeamRepo) List(ctx context.Context, limit, offset int) ([]model.Team, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var out []model.Team
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, err
}

func (r *gormTeamRepo) Detail(ctx context.Context, id uuid.UUID) (*model.TeamDetail, error) {
	t, err := r.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	var mgrs []model.TeamManager
	var mems []model.TeamMember
	if err := r.db.WithContext(ctx).Where("team_id = ?", id).Find(&mgrs).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Where("team_id = ?", id).Find(&mems).Error; err != nil {
		return nil, err
	}
	return &model.TeamDetail{Team: *t, Managers: mgrs, Members: mems}, nil
}

func (r *gormTeamRepo) IsManager(ctx context.Context, teamID, uid uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.TeamManager{}).Where("team_id = ? AND user_id = ?", teamID, uid).Count(&n).Error
	return n > 0, err
}

func (r *gormTeamRepo) IsMainManager(ctx context.Context, teamID, uid uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.TeamManager{}).Where("team_id = ? AND user_id = ? AND is_main", teamID, uid).Count(&n).Error
	return n > 0, err
}

func (r *gormTeamRepo) IsMember(ctx context.Context, teamID, uid uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.TeamMember{}).Where("team_id = ? AND user_id = ?", teamID, uid).Count(&n).Error
	return n > 0, err
}

func (r *gormTeamRepo) AddMember(ctx context.Context, teamID, uid uuid.UUID) error {
	m := &model.TeamMember{TeamID: teamID, UserID: uid}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isUnique(err) {
			return httpx.ErrConflict
		}
		return err
	}
	return nil
}

func (r *gormTeamRepo) RemoveMember(ctx context.Context, teamID, uid uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, uid).Delete(&model.TeamMember{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (r *gormTeamRepo) AddManager(ctx context.Context, teamID, uid uuid.UUID) error {
	m := &model.TeamManager{TeamID: teamID, UserID: uid}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isUnique(err) {
			return httpx.ErrConflict
		}
		return err
	}
	return nil
}

func (r *gormTeamRepo) RemoveManager(ctx context.Context, teamID, uid uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, uid).Delete(&model.TeamManager{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (r *gormTeamRepo) CountMainManagers(ctx context.Context, teamID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.TeamManager{}).Where("team_id = ? AND is_main", teamID).Count(&n).Error
	return n, err
}

func (r *gormTeamRepo) ManagersOf(ctx context.Context, uid uuid.UUID) ([]uuid.UUID, error) {
	const q = `
SELECT DISTINCT tm_mgr.user_id
FROM team_members tm_mem
JOIN team_managers tm_mgr ON tm_mgr.team_id = tm_mem.team_id
WHERE tm_mem.user_id = ?`
	var out []uuid.UUID
	if err := r.db.WithContext(ctx).Raw(q, uid).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}
