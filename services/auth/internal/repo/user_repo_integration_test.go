//go:build integration

package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/auth/internal/model"
	"github.com/hieu-seta/seta-training/services/auth/internal/repo"
	"github.com/hieu-seta/seta-training/services/auth/internal/testutil"
)

func TestUserRepo_Integration(t *testing.T) {
	db, _ := testutil.Stack(t, "auth")
	testutil.ApplyAuthMigration(t, db)
	defer testutil.TruncateAll(t, db)
	r := repo.NewUserRepo(db)
	ctx := context.Background()

	u := &model.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "fake_hash",
		Role:         model.RoleMember,
	}

	t.Run("Create + ByEmail + ByID + Exists", func(t *testing.T) {
		if err := r.Create(ctx, u); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := r.ByEmail(ctx, "alice@example.com")
		if err != nil {
			t.Fatalf("by email: %v", err)
		}
		if got.ID != u.ID {
			t.Errorf("id mismatch")
		}
		got2, err := r.ByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("by id: %v", err)
		}
		if got2.Email != u.Email {
			t.Errorf("email mismatch")
		}
		ok, err := r.Exists(ctx, u.ID)
		if err != nil || !ok {
			t.Errorf("exists got %v %v", ok, err)
		}
	})

	t.Run("Duplicate email → ErrConflict", func(t *testing.T) {
		dup := *u
		dup.ID = uuid.New()
		err := r.Create(ctx, &dup)
		if !errors.Is(err, httpx.ErrConflict) {
			t.Errorf("want conflict, got %v", err)
		}
	})

	t.Run("ByEmail missing → ErrNotFound", func(t *testing.T) {
		_, err := r.ByEmail(ctx, "ghost@example.com")
		if !errors.Is(err, httpx.ErrNotFound) {
			t.Errorf("want not found, got %v", err)
		}
	})

	t.Run("Exists missing → false", func(t *testing.T) {
		ok, err := r.Exists(ctx, uuid.New())
		if err != nil || ok {
			t.Errorf("want false nil, got %v %v", ok, err)
		}
	})

	t.Run("List paginates", func(t *testing.T) {
		// add 3 more
		for i := 0; i < 3; i++ {
			extra := &model.User{
				ID:           uuid.New(),
				Username:     "u",
				Email:        uuid.New().String() + "@e.f",
				PasswordHash: "x",
				Role:         model.RoleMember,
			}
			_ = r.Create(ctx, extra)
		}
		list, err := r.List(ctx, 2, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("want 2, got %d", len(list))
		}
	})
}
