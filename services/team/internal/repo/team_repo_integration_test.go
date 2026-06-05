//go:build integration

package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/team/internal/model"
	"github.com/hieu-seta/seta-training/services/team/internal/repo"
	"github.com/hieu-seta/seta-training/services/team/internal/testutil"
)

func TestTeamRepo_Integration(t *testing.T) {
	db := testutil.StartPG(t)
	defer testutil.Truncate(t, db)
	r := repo.New(db)
	ctx := context.Background()

	creator := uuid.New()
	team := &model.Team{ID: uuid.New(), Name: "Eng", CreatedBy: creator}

	t.Run("Create + main mgr seeded in same tx", func(t *testing.T) {
		if err := r.Create(ctx, team, creator); err != nil {
			t.Fatalf("create: %v", err)
		}
		isMain, err := r.IsMainManager(ctx, team.ID, creator)
		if err != nil || !isMain {
			t.Errorf("creator not main: %v err=%v", isMain, err)
		}
	})

	t.Run("AddMember + IsMember", func(t *testing.T) {
		u := uuid.New()
		if err := r.AddMember(ctx, team.ID, u); err != nil {
			t.Fatalf("add: %v", err)
		}
		ok, _ := r.IsMember(ctx, team.ID, u)
		if !ok {
			t.Error("not a member")
		}
	})

	t.Run("AddMember duplicate → 409 conflict", func(t *testing.T) {
		u := uuid.New()
		_ = r.AddMember(ctx, team.ID, u)
		if err := r.AddMember(ctx, team.ID, u); !errors.Is(err, httpx.ErrConflict) {
			t.Errorf("want conflict, got %v", err)
		}
	})

	t.Run("Only one main manager (unique partial idx)", func(t *testing.T) {
		// Try to add another main mgr directly via SQL.
		other := uuid.New()
		err := db.Exec("INSERT INTO team_managers (team_id, user_id, is_main) VALUES (?, ?, true)", team.ID, other).Error
		if err == nil {
			t.Errorf("expected unique idx violation when adding second main mgr")
		}
	})

	t.Run("RemoveMember not present → 404", func(t *testing.T) {
		err := r.RemoveMember(ctx, team.ID, uuid.New())
		if !errors.Is(err, httpx.ErrNotFound) {
			t.Errorf("want not found, got %v", err)
		}
	})

	t.Run("Detail returns members + managers", func(t *testing.T) {
		d, err := r.Detail(ctx, team.ID)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if len(d.Managers) == 0 {
			t.Errorf("expected managers, got 0")
		}
	})

	t.Run("AddManager non-main allowed", func(t *testing.T) {
		other := uuid.New()
		if err := r.AddManager(ctx, team.ID, other); err != nil {
			t.Fatalf("add mgr: %v", err)
		}
		isMgr, _ := r.IsManager(ctx, team.ID, other)
		isMain, _ := r.IsMainManager(ctx, team.ID, other)
		if !isMgr || isMain {
			t.Errorf("got mgr=%v main=%v", isMgr, isMain)
		}
	})

	t.Run("CountMainManagers = 1", func(t *testing.T) {
		n, err := r.CountMainManagers(ctx, team.ID)
		if err != nil || n != 1 {
			t.Errorf("want 1, got %d err=%v", n, err)
		}
	})
}
