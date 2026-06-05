package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/cache"
)

func TestKeys_Stable(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	if cache.TeamMembersKey(id) != "team:11111111-2222-3333-4444-555555555555:members" {
		t.Errorf("TeamMembersKey shape changed")
	}
	if cache.FolderMetaKey(id) != "asset:folder:11111111-2222-3333-4444-555555555555" {
		t.Errorf("FolderMetaKey shape changed")
	}
	if cache.ACLKey(id, "note", "abc", "read") != "acl:11111111-2222-3333-4444-555555555555:note:abc:read" {
		t.Errorf("ACLKey shape changed")
	}
}

func TestNoop_MissAlways(t *testing.T) {
	var n cache.Noop
	var out struct{ X int }
	err := n.GetJSON(context.Background(), "anything", &out)
	if !errors.Is(err, cache.ErrMiss) {
		t.Errorf("Noop.GetJSON should ErrMiss, got %v", err)
	}
	if err := n.SetJSON(context.Background(), "k", "v", time.Minute); err != nil {
		t.Errorf("Noop.SetJSON should swallow, got %v", err)
	}
	if err := n.Del(context.Background(), "k"); err != nil {
		t.Errorf("Noop.Del should swallow, got %v", err)
	}
}
