//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/services/asset/internal/model"
	"github.com/hieu-seta/seta-training/services/asset/internal/repo"
	"github.com/hieu-seta/seta-training/services/asset/internal/testutil"
)

func TestRepos_Integration(t *testing.T) {
	db := testutil.StartPG(t)
	fr := repo.NewFolderRepo(db)
	nr := repo.NewNoteRepo(db)
	ctx := context.Background()

	owner := uuid.New()
	sharee := uuid.New()
	other := uuid.New()

	f := &model.Folder{ID: uuid.New(), OwnerID: owner, Name: "F1"}
	if err := fr.Create(ctx, f); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	n := &model.Note{ID: uuid.New(), FolderID: f.ID, OwnerID: owner, Title: "N1", Body: "body"}
	if err := nr.Create(ctx, n); err != nil {
		t.Fatalf("create note: %v", err)
	}

	t.Run("Folder ShareAccess returns access on share, empty otherwise", func(t *testing.T) {
		if err := fr.Share(ctx, &model.FolderShare{FolderID: f.ID, UserID: sharee, Access: model.AccessRead}); err != nil {
			t.Fatalf("share: %v", err)
		}
		acc, err := fr.ShareAccess(ctx, f.ID, sharee)
		if err != nil || acc != model.AccessRead {
			t.Errorf("want read, got %q err=%v", acc, err)
		}
		acc, _ = fr.ShareAccess(ctx, f.ID, other)
		if acc != "" {
			t.Errorf("want empty, got %q", acc)
		}
	})

	t.Run("Note ShareAccess: folder-share grants note read", func(t *testing.T) {
		// Sharee already has folder read. Note has no direct share. → expect "read".
		acc, err := nr.ShareAccess(ctx, n.ID, sharee)
		if err != nil {
			t.Fatalf("share access: %v", err)
		}
		if acc != model.AccessRead {
			t.Errorf("want read via folder inheritance, got %q", acc)
		}
	})

	t.Run("Note ShareAccess: note-write beats folder-read", func(t *testing.T) {
		if err := nr.Share(ctx, &model.NoteShare{NoteID: n.ID, UserID: sharee, Access: model.AccessWrite}); err != nil {
			t.Fatalf("note share: %v", err)
		}
		acc, _ := nr.ShareAccess(ctx, n.ID, sharee)
		if acc != model.AccessWrite {
			t.Errorf("want write, got %q", acc)
		}
	})

	t.Run("Unshare folder removes access", func(t *testing.T) {
		if err := fr.Unshare(ctx, f.ID, sharee); err != nil {
			t.Fatalf("unshare: %v", err)
		}
		acc, _ := fr.ShareAccess(ctx, f.ID, sharee)
		if acc != "" {
			t.Errorf("expected revoked, got %q", acc)
		}
	})

	t.Run("ListVisible returns owned + shared", func(t *testing.T) {
		// Add a second folder owned by other; share it w/ sharee.
		f2 := &model.Folder{ID: uuid.New(), OwnerID: other, Name: "F2"}
		_ = fr.Create(ctx, f2)
		_ = fr.Share(ctx, &model.FolderShare{FolderID: f2.ID, UserID: sharee, Access: model.AccessRead})
		// sharee should see f2 (shared). f (no share after unshare) — should NOT be visible.
		list, err := fr.ListVisible(ctx, sharee, nil, 50, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		seen := map[uuid.UUID]bool{}
		for _, x := range list {
			seen[x.ID] = true
		}
		if !seen[f2.ID] {
			t.Errorf("expected f2 visible to sharee")
		}
		if seen[f.ID] {
			t.Errorf("f1 should NOT be visible after unshare")
		}
	})

	t.Run("Bad access value rejected by CHECK", func(t *testing.T) {
		err := fr.Share(ctx, &model.FolderShare{FolderID: f.ID, UserID: other, Access: "admin"})
		if err == nil {
			t.Errorf("expected error for bad access value")
		}
	})

	t.Run("Cascade delete: folder removes its notes + shares", func(t *testing.T) {
		ff := &model.Folder{ID: uuid.New(), OwnerID: owner, Name: "FX"}
		_ = fr.Create(ctx, ff)
		nn := &model.Note{ID: uuid.New(), FolderID: ff.ID, OwnerID: owner, Title: "X"}
		_ = nr.Create(ctx, nn)
		_ = fr.Delete(ctx, ff.ID)
		if _, err := nr.ByID(ctx, nn.ID); err == nil {
			t.Errorf("note should be cascaded")
		}
	})
}
