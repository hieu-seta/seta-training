package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/team/internal/service"
)

func newSvc() (*service.TeamService, *fakeTeamRepo, *fakeAuthClient) {
	r := newFakeTeamRepo()
	a := newFakeAuth()
	return service.New(r, a, nil), r, a
}

func TestCreate_CallerBecomesMainManager(t *testing.T) {
	s, r, _ := newSvc()
	creator := uuid.New()
	team, err := s.Create(context.Background(), "Engineering", creator)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if team.CreatedBy != creator {
		t.Errorf("created_by mismatch")
	}
	if isMain, _ := r.IsMainManager(context.Background(), team.ID, creator); !isMain {
		t.Errorf("creator is not main mgr")
	}
}

func TestCreate_InvalidName(t *testing.T) {
	s, _, _ := newSvc()
	for _, n := range []string{"", "   ", string(make([]byte, 65))} {
		_, err := s.Create(context.Background(), n, uuid.New())
		if !errors.Is(err, httpx.ErrBadRequest) {
			t.Errorf("name=%q: want bad request, got %v", n, err)
		}
	}
}

func TestAddMember_NonMgr_Forbidden(t *testing.T) {
	s, _, _ := newSvc()
	team, _ := s.Create(context.Background(), "A", uuid.New())
	other := uuid.New()
	err := s.AddMember(context.Background(), team.ID, other, uuid.New())
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
}

func TestAddMember_TargetMissingInAuth_NotFound(t *testing.T) {
	s, _, _ := newSvc()
	creator := uuid.New()
	team, _ := s.Create(context.Background(), "A", creator)
	// auth has no users set → returns false.
	err := s.AddMember(context.Background(), team.ID, creator, uuid.New())
	if !errors.Is(err, httpx.ErrNotFound) {
		t.Errorf("want not found, got %v", err)
	}
}

func TestAddMember_AuthSvcDown_Unavailable(t *testing.T) {
	s, _, a := newSvc()
	creator := uuid.New()
	team, _ := s.Create(context.Background(), "A", creator)
	a.failure = fmt.Errorf("%w: dial tcp", httpx.ErrUnavailable)
	err := s.AddMember(context.Background(), team.ID, creator, uuid.New())
	if !errors.Is(err, httpx.ErrUnavailable) {
		t.Errorf("want unavailable, got %v", err)
	}
}

func TestAddMember_Happy(t *testing.T) {
	s, r, a := newSvc()
	creator := uuid.New()
	team, _ := s.Create(context.Background(), "A", creator)
	target := uuid.New()
	a.exists[target] = true
	if err := s.AddMember(context.Background(), team.ID, creator, target); err != nil {
		t.Fatalf("add: %v", err)
	}
	if ok, _ := r.IsMember(context.Background(), team.ID, target); !ok {
		t.Errorf("target not added")
	}
}

func TestAddManager_NonMainMgr_Forbidden(t *testing.T) {
	s, r, a := newSvc()
	main := uuid.New()
	other := uuid.New()
	team, _ := s.Create(context.Background(), "A", main)
	// promote other to non-main mgr.
	_ = r.AddManager(context.Background(), team.ID, other)
	target := uuid.New()
	a.exists[target] = true
	err := s.AddManager(context.Background(), team.ID, other, target)
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden (non-main), got %v", err)
	}
}

func TestAddManager_MainMgr_Happy(t *testing.T) {
	s, _, a := newSvc()
	main := uuid.New()
	team, _ := s.Create(context.Background(), "A", main)
	target := uuid.New()
	a.exists[target] = true
	if err := s.AddManager(context.Background(), team.ID, main, target); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestRemoveManager_LastMain_Forbidden(t *testing.T) {
	s, _, _ := newSvc()
	main := uuid.New()
	team, _ := s.Create(context.Background(), "A", main)
	err := s.RemoveManager(context.Background(), team.ID, main, main)
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden last-main, got %v", err)
	}
}

func TestRemoveManager_NonMain_OK(t *testing.T) {
	s, r, _ := newSvc()
	main := uuid.New()
	team, _ := s.Create(context.Background(), "A", main)
	other := uuid.New()
	_ = r.AddManager(context.Background(), team.ID, other)
	if err := s.RemoveManager(context.Background(), team.ID, main, other); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestDetail_NonMemberNorMgr_Forbidden(t *testing.T) {
	s, _, _ := newSvc()
	main := uuid.New()
	team, _ := s.Create(context.Background(), "A", main)
	_, err := s.Detail(context.Background(), team.ID, uuid.New())
	if !errors.Is(err, httpx.ErrForbidden) {
		t.Errorf("want forbidden, got %v", err)
	}
}

func TestDetail_MgrSees(t *testing.T) {
	s, _, _ := newSvc()
	main := uuid.New()
	team, _ := s.Create(context.Background(), "A", main)
	d, err := s.Detail(context.Background(), team.ID, main)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.ID != team.ID || len(d.Managers) != 1 {
		t.Errorf("unexpected detail: %+v", d)
	}
}
