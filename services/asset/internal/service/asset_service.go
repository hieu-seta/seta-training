package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/logger"
	"github.com/hieu-seta/seta-training/pkg/teamclient"
	"github.com/hieu-seta/seta-training/services/asset/internal/model"
	"github.com/hieu-seta/seta-training/services/asset/internal/repo"
)

// AssetService orchestrates folders, notes, and shares behind ACL gates.
type AssetService struct {
	folders repo.FolderRepo
	notes   repo.NoteRepo
	team    teamclient.TeamClient
	pub     events.Publisher
	acl     *ACL
	now     func() time.Time
	newID   func() uuid.UUID
}

// New builds an AssetService.
func New(f repo.FolderRepo, n repo.NoteRepo, t teamclient.TeamClient, pub events.Publisher) *AssetService {
	if pub == nil {
		pub = events.Noop{}
	}
	return &AssetService{
		folders: f, notes: n, team: t, pub: pub,
		acl: NewACL(f, n, t), now: time.Now, newID: uuid.New,
	}
}

func (s *AssetService) emit(ctx context.Context, subj string, payload any, msgID string) {
	_ = s.pub.Publish(ctx, subj, payload, msgID)
}

func (s *AssetService) emitAsset(ctx context.Context, subj string, asset, owner, folderID, actor uuid.UUID) {
	s.emit(ctx, subj, events.AssetChanged{
		Envelope: s.envelope(ctx, subj),
		AssetID:  asset, OwnerID: owner, FolderID: folderID, Actor: actor,
	}, events.MsgID(subj, asset))
}

func (s *AssetService) emitShare(ctx context.Context, subj string, asset, owner, target, actor uuid.UUID, access string) {
	s.emit(ctx, subj, events.ShareChanged{
		Envelope: s.envelope(ctx, subj),
		AssetID:  asset, OwnerID: owner, UserID: target, Access: access, Actor: actor,
	}, events.MsgID(subj, asset, target))
}

func (s *AssetService) envelope(ctx context.Context, eventType string) events.Envelope {
	return events.Envelope{
		Type:       eventType,
		OccurredAt: s.now(),
		RequestID:  logger.ReqIDFromContext(ctx),
	}
}

// CreateFolder — caller becomes owner.
func (s *AssetService) CreateFolder(ctx context.Context, name string, owner uuid.UUID) (*model.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, fmt.Errorf("%w: folder name 1..200", httpx.ErrBadRequest)
	}
	f := &model.Folder{ID: s.newID(), OwnerID: owner, Name: name}
	if err := s.folders.Create(ctx, f); err != nil {
		return nil, err
	}
	s.emitAsset(ctx, events.SubjFolderCreated, f.ID, owner, uuid.Nil, owner)
	return f, nil
}

// GetFolder — ACL: read.
func (s *AssetService) GetFolder(ctx context.Context, id, caller uuid.UUID) (*model.Folder, error) {
	if err := s.acl.Check(ctx, caller, KindFolder, id, OpRead); err != nil {
		return nil, err
	}
	return s.folders.ByID(ctx, id)
}

// UpdateFolder — ACL: write.
func (s *AssetService) UpdateFolder(ctx context.Context, id, caller uuid.UUID, name string) (*model.Folder, error) {
	if err := s.acl.Check(ctx, caller, KindFolder, id, OpWrite); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, fmt.Errorf("%w: name", httpx.ErrBadRequest)
	}
	f, err := s.folders.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	f.Name = name
	if err := s.folders.Update(ctx, f); err != nil {
		return nil, err
	}
	s.emitAsset(ctx, events.SubjFolderUpdated, f.ID, f.OwnerID, uuid.Nil, caller)
	return f, nil
}

// DeleteFolder — owner only.
func (s *AssetService) DeleteFolder(ctx context.Context, id, caller uuid.UUID) error {
	f, err := s.folders.ByID(ctx, id)
	if err != nil {
		return err
	}
	if f.OwnerID != caller {
		return httpx.ErrForbidden
	}
	if err := s.folders.Delete(ctx, id); err != nil {
		return err
	}
	s.emitAsset(ctx, events.SubjFolderDeleted, id, f.OwnerID, uuid.Nil, caller)
	return nil
}

// ListFolders — owned + explicitly shared.
// Manager oversight is GET-only (per stage 2 spec); list doesn't enumerate other users' folders.
func (s *AssetService) ListFolders(ctx context.Context, caller uuid.UUID, limit, offset int) ([]model.Folder, error) {
	return s.folders.ListVisible(ctx, caller, nil, limit, offset)
}

// ShareFolder grants access. Caller must own.
func (s *AssetService) ShareFolder(ctx context.Context, folderID, caller, target uuid.UUID, access string) error {
	if !model.IsValidAccess(access) {
		return fmt.Errorf("%w: access must be read|write", httpx.ErrBadRequest)
	}
	f, err := s.folders.ByID(ctx, folderID)
	if err != nil {
		return err
	}
	if f.OwnerID != caller {
		return httpx.ErrForbidden
	}
	if target == caller {
		return fmt.Errorf("%w: cannot share w/ self", httpx.ErrBadRequest)
	}
	share := &model.FolderShare{FolderID: folderID, UserID: target, Access: access}
	if err := s.folders.Share(ctx, share); err != nil {
		return err
	}
	s.emitShare(ctx, events.SubjFolderShared, folderID, f.OwnerID, target, caller, access)
	return nil
}

// UnshareFolder — owner only.
func (s *AssetService) UnshareFolder(ctx context.Context, folderID, caller, target uuid.UUID) error {
	f, err := s.folders.ByID(ctx, folderID)
	if err != nil {
		return err
	}
	if f.OwnerID != caller {
		return httpx.ErrForbidden
	}
	if err := s.folders.Unshare(ctx, folderID, target); err != nil {
		return err
	}
	s.emitShare(ctx, events.SubjFolderRevoked, folderID, f.OwnerID, target, caller, "")
	return nil
}

// CreateNote — caller must be able to WRITE the folder.
func (s *AssetService) CreateNote(ctx context.Context, folderID, caller uuid.UUID, title, body string) (*model.Note, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 200 {
		return nil, fmt.Errorf("%w: title 1..200", httpx.ErrBadRequest)
	}
	if len(body) > 64*1024 {
		return nil, fmt.Errorf("%w: body >64KB", httpx.ErrBadRequest)
	}
	if err := s.acl.Check(ctx, caller, KindFolder, folderID, OpWrite); err != nil {
		return nil, err
	}
	n := &model.Note{ID: s.newID(), FolderID: folderID, OwnerID: caller, Title: title, Body: body}
	if err := s.notes.Create(ctx, n); err != nil {
		return nil, err
	}
	s.emitAsset(ctx, events.SubjNoteCreated, n.ID, caller, folderID, caller)
	return n, nil
}

// GetNote — ACL: read.
func (s *AssetService) GetNote(ctx context.Context, id, caller uuid.UUID) (*model.Note, error) {
	if err := s.acl.Check(ctx, caller, KindNote, id, OpRead); err != nil {
		return nil, err
	}
	return s.notes.ByID(ctx, id)
}

// UpdateNote — ACL: write.
func (s *AssetService) UpdateNote(ctx context.Context, id, caller uuid.UUID, title, body string) (*model.Note, error) {
	if err := s.acl.Check(ctx, caller, KindNote, id, OpWrite); err != nil {
		return nil, err
	}
	n, err := s.notes.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if title != "" {
		if len(title) > 200 {
			return nil, fmt.Errorf("%w: title", httpx.ErrBadRequest)
		}
		n.Title = title
	}
	if len(body) > 64*1024 {
		return nil, fmt.Errorf("%w: body >64KB", httpx.ErrBadRequest)
	}
	n.Body = body
	if err := s.notes.Update(ctx, n); err != nil {
		return nil, err
	}
	s.emitAsset(ctx, events.SubjNoteUpdated, n.ID, n.OwnerID, n.FolderID, caller)
	return n, nil
}

// DeleteNote — owner only.
func (s *AssetService) DeleteNote(ctx context.Context, id, caller uuid.UUID) error {
	n, err := s.notes.ByID(ctx, id)
	if err != nil {
		return err
	}
	if n.OwnerID != caller {
		return httpx.ErrForbidden
	}
	if err := s.notes.Delete(ctx, id); err != nil {
		return err
	}
	s.emitAsset(ctx, events.SubjNoteDeleted, id, n.OwnerID, n.FolderID, caller)
	return nil
}

// ShareNote — owner only.
func (s *AssetService) ShareNote(ctx context.Context, noteID, caller, target uuid.UUID, access string) error {
	if !model.IsValidAccess(access) {
		return fmt.Errorf("%w: access", httpx.ErrBadRequest)
	}
	n, err := s.notes.ByID(ctx, noteID)
	if err != nil {
		return err
	}
	if n.OwnerID != caller {
		return httpx.ErrForbidden
	}
	if target == caller {
		return fmt.Errorf("%w: cannot share w/ self", httpx.ErrBadRequest)
	}
	if err := s.notes.Share(ctx, &model.NoteShare{NoteID: noteID, UserID: target, Access: access}); err != nil {
		return err
	}
	s.emitShare(ctx, events.SubjNoteShared, noteID, n.OwnerID, target, caller, access)
	return nil
}

// UnshareNote — owner only.
func (s *AssetService) UnshareNote(ctx context.Context, noteID, caller, target uuid.UUID) error {
	n, err := s.notes.ByID(ctx, noteID)
	if err != nil {
		return err
	}
	if n.OwnerID != caller {
		return httpx.ErrForbidden
	}
	if err := s.notes.Unshare(ctx, noteID, target); err != nil {
		return err
	}
	s.emitShare(ctx, events.SubjNoteRevoked, noteID, n.OwnerID, target, caller, "")
	return nil
}
