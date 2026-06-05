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

// NoteRepo = persistence port for notes + note_shares.
type NoteRepo interface {
	Create(ctx context.Context, n *model.Note) error
	ByID(ctx context.Context, id uuid.UUID) (*model.Note, error)
	Update(ctx context.Context, n *model.Note) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListInFolder(ctx context.Context, folderID uuid.UUID) ([]model.Note, error)

	Share(ctx context.Context, s *model.NoteShare) error
	Unshare(ctx context.Context, noteID, uid uuid.UUID) error
	// ShareAccess returns the strongest access (note-share OR parent-folder-share) for uid on note.
	ShareAccess(ctx context.Context, noteID, uid uuid.UUID) (string, error)
}

type gormNoteRepo struct{ db *gorm.DB }

// NewNoteRepo builds a GORM-backed NoteRepo.
func NewNoteRepo(db *gorm.DB) NoteRepo { return &gormNoteRepo{db: db} }

func (r *gormNoteRepo) Create(ctx context.Context, n *model.Note) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *gormNoteRepo) ByID(ctx context.Context, id uuid.UUID) (*model.Note, error) {
	var n model.Note
	err := r.db.WithContext(ctx).First(&n, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *gormNoteRepo) Update(ctx context.Context, n *model.Note) error {
	return r.db.WithContext(ctx).Save(n).Error
}

func (r *gormNoteRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Note{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (r *gormNoteRepo) ListInFolder(ctx context.Context, folderID uuid.UUID) ([]model.Note, error) {
	var out []model.Note
	err := r.db.WithContext(ctx).Where("folder_id = ?", folderID).Order("created_at DESC").Find(&out).Error
	return out, err
}

func (r *gormNoteRepo) Share(ctx context.Context, s *model.NoteShare) error {
	err := r.db.WithContext(ctx).Save(s).Error
	if err != nil && strings.Contains(err.Error(), "violates check") {
		return httpx.ErrBadRequest
	}
	return err
}

func (r *gormNoteRepo) Unshare(ctx context.Context, noteID, uid uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("note_id = ? AND user_id = ?", noteID, uid).Delete(&model.NoteShare{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// ShareAccess: UNION of (note share, parent folder share). "write" beats "read".
func (r *gormNoteRepo) ShareAccess(ctx context.Context, noteID, uid uuid.UUID) (string, error) {
	const q = `
SELECT access FROM (
    SELECT access FROM note_shares   WHERE note_id   = ? AND user_id = ?
    UNION ALL
    SELECT fs.access FROM folder_shares fs JOIN notes n ON n.folder_id = fs.folder_id
        WHERE n.id = ? AND fs.user_id = ?
) all_grants
ORDER BY CASE access WHEN 'write' THEN 1 WHEN 'read' THEN 2 ELSE 3 END
LIMIT 1`
	var access string
	err := r.db.WithContext(ctx).Raw(q, noteID, uid, noteID, uid).Scan(&access).Error
	if err != nil {
		return "", err
	}
	return access, nil // "" if none
}
