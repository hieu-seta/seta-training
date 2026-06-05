package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/asset/internal/model"
	"github.com/hieu-seta/seta-training/services/asset/internal/service"
)

func newSvc(t *testing.T) (*service.AssetService, *fakeFolderRepo, *fakeNoteRepo, *fakeTeamClient) {
	t.Helper()
	fr := newFakeFolderRepo()
	nr := newFakeNoteRepo(fr)
	tc := newFakeTeamClient()
	return service.New(fr, nr, tc, nil), fr, nr, tc
}

func TestCreateFolder_OwnerSet(t *testing.T) {
	s, _, _, _ := newSvc(t)
	owner := uuid.New()
	f, err := s.CreateFolder(context.Background(), "My Folder", owner)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if f.OwnerID != owner {
		t.Errorf("owner mismatch")
	}
}

func TestCreateFolder_BadName(t *testing.T) {
	s, _, _, _ := newSvc(t)
	_, err := s.CreateFolder(context.Background(), "", uuid.New())
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Errorf("want bad request, got %v", err)
	}
}

func TestUpdateFolder_NonOwner_Forbidden(t *testing.T) {
	s, _, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	_, err := s.UpdateFolder(context.Background(), f.ID, uuid.New(), "renamed")
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
}

func TestUpdateFolder_WriteShare_OK(t *testing.T) {
	s, fr, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	sharee := uuid.New()
	_ = fr.Share(context.Background(), &model.FolderShare{FolderID: f.ID, UserID: sharee, Access: model.AccessWrite})
	updated, err := s.UpdateFolder(context.Background(), f.ID, sharee, "ren")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "ren" {
		t.Errorf("name not updated")
	}
}

func TestDeleteFolder_OnlyOwner(t *testing.T) {
	s, fr, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	sharee := uuid.New()
	_ = fr.Share(context.Background(), &model.FolderShare{FolderID: f.ID, UserID: sharee, Access: model.AccessWrite})
	// Write share doesn't allow delete.
	if err := s.DeleteFolder(context.Background(), f.ID, sharee); !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
	if err := s.DeleteFolder(context.Background(), f.ID, owner); err != nil {
		t.Errorf("owner delete: %v", err)
	}
}

func TestShareFolder_OnlyOwner(t *testing.T) {
	s, _, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	err := s.ShareFolder(context.Background(), f.ID, uuid.New(), uuid.New(), model.AccessRead)
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
}

func TestShareFolder_BadAccess(t *testing.T) {
	s, _, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	err := s.ShareFolder(context.Background(), f.ID, owner, uuid.New(), "admin")
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Errorf("want bad req, got %v", err)
	}
}

func TestCreateNote_RequiresFolderWrite(t *testing.T) {
	s, _, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	// Non-owner cannot create.
	_, err := s.CreateNote(context.Background(), f.ID, uuid.New(), "T", "")
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
}

func TestUpdateNote_NoteShareWrite_OK(t *testing.T) {
	s, _, nr, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	n, _ := s.CreateNote(context.Background(), f.ID, owner, "T", "")
	sharee := uuid.New()
	_ = nr.Share(context.Background(), &model.NoteShare{NoteID: n.ID, UserID: sharee, Access: model.AccessWrite})
	if _, err := s.UpdateNote(context.Background(), n.ID, sharee, "T2", "body"); err != nil {
		t.Errorf("note-write share update: %v", err)
	}
}

func TestManagerOversight_ReadOnly_OnNotes(t *testing.T) {
	s, _, _, tc := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	n, _ := s.CreateNote(context.Background(), f.ID, owner, "T", "secret")
	mgr := uuid.New()
	tc.mgrsOf[owner] = []uuid.UUID{mgr}
	// Read OK
	if _, err := s.GetNote(context.Background(), n.ID, mgr); err != nil {
		t.Errorf("mgr read: %v", err)
	}
	// Write 403
	if _, err := s.UpdateNote(context.Background(), n.ID, mgr, "T'", "tamper"); !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("mgr should not write: %v", err)
	}
}

func TestListFolders_OwnedPlusShared(t *testing.T) {
	s, fr, _, _ := newSvc(t)
	a := uuid.New()
	b := uuid.New()
	f1, _ := s.CreateFolder(context.Background(), "F1", a)
	f2, _ := s.CreateFolder(context.Background(), "F2", b)
	_, _ = s.CreateFolder(context.Background(), "F3", b)
	_ = fr.Share(context.Background(), &model.FolderShare{FolderID: f2.ID, UserID: a, Access: model.AccessRead})

	list, err := s.ListFolders(context.Background(), a, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Should see f1 (owned) + f2 (shared); NOT f3.
	ids := map[uuid.UUID]bool{}
	for _, f := range list {
		ids[f.ID] = true
	}
	if !ids[f1.ID] || !ids[f2.ID] {
		t.Errorf("expected f1+f2, got %v", ids)
	}
}

func TestShareFolder_Self_BadRequest(t *testing.T) {
	s, _, _, _ := newSvc(t)
	owner := uuid.New()
	f, _ := s.CreateFolder(context.Background(), "F", owner)
	err := s.ShareFolder(context.Background(), f.ID, owner, owner, model.AccessRead)
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Errorf("want bad req, got %v", err)
	}
}
