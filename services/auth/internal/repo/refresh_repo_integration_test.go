//go:build integration

package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/auth/internal/repo"
	"github.com/hieu-seta/seta-training/services/auth/internal/testutil"
)

func TestRefreshRepo_Integration(t *testing.T) {
	_, rdb := testutil.Stack(t, "auth")
	r := repo.NewRefreshRepo(rdb)
	ctx := context.Background()

	t.Cleanup(func() { testutil.FlushRedis(t, rdb) })

	t.Run("Store + Lookup", func(t *testing.T) {
		testutil.FlushRedis(t, rdb)
		if err := r.Store(ctx, "j1", "u1", "fam-a", time.Minute); err != nil {
			t.Fatalf("store: %v", err)
		}
		uid, fam, err := r.Lookup(ctx, "j1")
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if uid != "u1" || fam != "fam-a" {
			t.Errorf("got uid=%s fam=%s", uid, fam)
		}
	})

	t.Run("Lookup missing → ErrUnauthd", func(t *testing.T) {
		testutil.FlushRedis(t, rdb)
		_, _, err := r.Lookup(ctx, "missing")
		if !errors.Is(err, httpx.ErrUnauthd) {
			t.Errorf("want unauthd, got %v", err)
		}
	})

	t.Run("Delete single", func(t *testing.T) {
		testutil.FlushRedis(t, rdb)
		_ = r.Store(ctx, "j2", "u1", "fam-b", time.Minute)
		if err := r.Delete(ctx, "j2"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, _, err := r.Lookup(ctx, "j2")
		if !errors.Is(err, httpx.ErrUnauthd) {
			t.Errorf("want unauthd after delete, got %v", err)
		}
	})

	t.Run("DeleteFamily wipes siblings", func(t *testing.T) {
		testutil.FlushRedis(t, rdb)
		_ = r.Store(ctx, "j3", "u", "fam-c", time.Minute)
		_ = r.Store(ctx, "j4", "u", "fam-c", time.Minute)
		_ = r.Store(ctx, "j5", "u", "other-fam", time.Minute)
		if err := r.DeleteFamily(ctx, "fam-c"); err != nil {
			t.Fatalf("delete family: %v", err)
		}
		if _, _, err := r.Lookup(ctx, "j3"); !errors.Is(err, httpx.ErrUnauthd) {
			t.Errorf("j3 should be gone")
		}
		if _, _, err := r.Lookup(ctx, "j4"); !errors.Is(err, httpx.ErrUnauthd) {
			t.Errorf("j4 should be gone")
		}
		// other family untouched
		if _, _, err := r.Lookup(ctx, "j5"); err != nil {
			t.Errorf("j5 should remain, got %v", err)
		}
	})

	t.Run("TTL respected", func(t *testing.T) {
		testutil.FlushRedis(t, rdb)
		_ = r.Store(ctx, "j6", "u", "fam-ttl", 100*time.Millisecond)
		time.Sleep(250 * time.Millisecond)
		_, _, err := r.Lookup(ctx, "j6")
		if !errors.Is(err, httpx.ErrUnauthd) {
			t.Errorf("expected expiry, got %v", err)
		}
	})
}
