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

type aclEnv struct {
	acl    *service.ACL
	fr     *fakeFolderRepo
	nr     *fakeNoteRepo
	tc     *fakeTeamClient
	owner  uuid.UUID
	folder *model.Folder
	note   *model.Note
}

func setup(t *testing.T) *aclEnv {
	t.Helper()
	fr := newFakeFolderRepo()
	nr := newFakeNoteRepo(fr)
	tc := newFakeTeamClient()
	acl := service.NewACL(fr, nr, tc)
	owner := uuid.New()
	f := &model.Folder{ID: uuid.New(), OwnerID: owner, Name: "f"}
	n := &model.Note{ID: uuid.New(), OwnerID: owner, FolderID: f.ID, Title: "n"}
	_ = fr.Create(context.Background(), f)
	_ = nr.Create(context.Background(), n)
	return &aclEnv{acl: acl, fr: fr, nr: nr, tc: tc, owner: owner, folder: f, note: n}
}

func TestACL_Owner_ReadWrite(t *testing.T) {
	e := setup(t)
	for _, op := range []service.Op{service.OpRead, service.OpWrite} {
		if err := e.acl.Check(context.Background(), e.owner, service.KindFolder, e.folder.ID, op); err != nil {
			t.Errorf("owner %s should be allowed: %v", op, err)
		}
		if err := e.acl.Check(context.Background(), e.owner, service.KindNote, e.note.ID, op); err != nil {
			t.Errorf("owner note %s should be allowed: %v", op, err)
		}
	}
}

func TestACL_Stranger_Forbidden(t *testing.T) {
	e := setup(t)
	err := e.acl.Check(context.Background(), uuid.New(), service.KindFolder, e.folder.ID, service.OpRead)
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
}

func TestACL_FolderShareRead_GrantsRead_DeniesWrite(t *testing.T) {
	e := setup(t)
	sharee := uuid.New()
	_ = e.fr.Share(context.Background(), &model.FolderShare{FolderID: e.folder.ID, UserID: sharee, Access: model.AccessRead})
	if err := e.acl.Check(context.Background(), sharee, service.KindFolder, e.folder.ID, service.OpRead); err != nil {
		t.Errorf("read share should grant read: %v", err)
	}
	if err := e.acl.Check(context.Background(), sharee, service.KindFolder, e.folder.ID, service.OpWrite); !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("read share should NOT grant write: %v", err)
	}
}

func TestACL_FolderShareWrite_GrantsBoth(t *testing.T) {
	e := setup(t)
	sharee := uuid.New()
	_ = e.fr.Share(context.Background(), &model.FolderShare{FolderID: e.folder.ID, UserID: sharee, Access: model.AccessWrite})
	for _, op := range []service.Op{service.OpRead, service.OpWrite} {
		if err := e.acl.Check(context.Background(), sharee, service.KindFolder, e.folder.ID, op); err != nil {
			t.Errorf("write share missed op %s: %v", op, err)
		}
	}
}

func TestACL_FolderShare_GrantsNoteRead_Inheritance(t *testing.T) {
	e := setup(t)
	sharee := uuid.New()
	_ = e.fr.Share(context.Background(), &model.FolderShare{FolderID: e.folder.ID, UserID: sharee, Access: model.AccessRead})
	if err := e.acl.Check(context.Background(), sharee, service.KindNote, e.note.ID, service.OpRead); err != nil {
		t.Errorf("folder share should grant note read: %v", err)
	}
}

func TestACL_FolderShareWrite_GrantsNoteWrite_Inheritance(t *testing.T) {
	e := setup(t)
	sharee := uuid.New()
	_ = e.fr.Share(context.Background(), &model.FolderShare{FolderID: e.folder.ID, UserID: sharee, Access: model.AccessWrite})
	if err := e.acl.Check(context.Background(), sharee, service.KindNote, e.note.ID, service.OpWrite); err != nil {
		t.Errorf("folder write share should grant note write: %v", err)
	}
}

func TestACL_NoteShareOnly(t *testing.T) {
	e := setup(t)
	sharee := uuid.New()
	_ = e.nr.Share(context.Background(), &model.NoteShare{NoteID: e.note.ID, UserID: sharee, Access: model.AccessRead})
	if err := e.acl.Check(context.Background(), sharee, service.KindNote, e.note.ID, service.OpRead); err != nil {
		t.Errorf("note share read should pass: %v", err)
	}
	// Folder check should still fail (note share doesn't grant folder access).
	if err := e.acl.Check(context.Background(), sharee, service.KindFolder, e.folder.ID, service.OpRead); !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("note share leaked to folder: %v", err)
	}
}

func TestACL_ManagerOversight_GrantsReadOnly(t *testing.T) {
	e := setup(t)
	mgr := uuid.New()
	e.tc.mgrsOf[e.owner] = []uuid.UUID{mgr}

	if err := e.acl.Check(context.Background(), mgr, service.KindFolder, e.folder.ID, service.OpRead); err != nil {
		t.Errorf("mgr should read: %v", err)
	}
	if err := e.acl.Check(context.Background(), mgr, service.KindFolder, e.folder.ID, service.OpWrite); !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("mgr should NOT write: %v", err)
	}
	// Also for notes.
	if err := e.acl.Check(context.Background(), mgr, service.KindNote, e.note.ID, service.OpRead); err != nil {
		t.Errorf("mgr should read note: %v", err)
	}
	if err := e.acl.Check(context.Background(), mgr, service.KindNote, e.note.ID, service.OpWrite); !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("mgr should NOT write note: %v", err)
	}
}

func TestACL_TeamSvcDown_Unavailable(t *testing.T) {
	e := setup(t)
	e.tc.err = httpx.ErrUnavailable
	err := e.acl.Check(context.Background(), uuid.New(), service.KindFolder, e.folder.ID, service.OpRead)
	if !errors.Is(err, httpx.ErrUnavailable) {
		t.Errorf("want unavailable, got %v", err)
	}
}

func TestACL_MissingAsset_NotFound(t *testing.T) {
	e := setup(t)
	err := e.acl.Check(context.Background(), e.owner, service.KindFolder, uuid.New(), service.OpRead)
	if !errors.Is(err, httpx.ErrNotFound) {
		t.Errorf("want not found, got %v", err)
	}
}

func TestACL_RevokeBlocks(t *testing.T) {
	e := setup(t)
	sharee := uuid.New()
	_ = e.fr.Share(context.Background(), &model.FolderShare{FolderID: e.folder.ID, UserID: sharee, Access: model.AccessRead})
	_ = e.fr.Unshare(context.Background(), e.folder.ID, sharee)
	err := e.acl.Check(context.Background(), sharee, service.KindFolder, e.folder.ID, service.OpRead)
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden after revoke, got %v", err)
	}
}
