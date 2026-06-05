package service_test

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/team/internal/model"
)

// fakeTeamRepo — in-memory.
type fakeTeamRepo struct {
	mu       sync.Mutex
	teams    map[uuid.UUID]*model.Team
	members  map[uuid.UUID]map[uuid.UUID]bool
	managers map[uuid.UUID]map[uuid.UUID]bool // true = is_main
}

func newFakeTeamRepo() *fakeTeamRepo {
	return &fakeTeamRepo{
		teams:    map[uuid.UUID]*model.Team{},
		members:  map[uuid.UUID]map[uuid.UUID]bool{},
		managers: map[uuid.UUID]map[uuid.UUID]bool{},
	}
}

func (r *fakeTeamRepo) Create(_ context.Context, t *model.Team, main uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams[t.ID] = t
	if r.managers[t.ID] == nil {
		r.managers[t.ID] = map[uuid.UUID]bool{}
	}
	r.managers[t.ID][main] = true
	return nil
}

func (r *fakeTeamRepo) ByID(_ context.Context, id uuid.UUID) (*model.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.teams[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *fakeTeamRepo) List(_ context.Context, _, _ int) ([]model.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.Team, 0, len(r.teams))
	for _, t := range r.teams {
		out = append(out, *t)
	}
	return out, nil
}

func (r *fakeTeamRepo) Detail(_ context.Context, id uuid.UUID) (*model.TeamDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.teams[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	d := &model.TeamDetail{Team: *t}
	for uid, main := range r.managers[id] {
		d.Managers = append(d.Managers, model.TeamManager{TeamID: id, UserID: uid, IsMain: main})
	}
	for uid := range r.members[id] {
		d.Members = append(d.Members, model.TeamMember{TeamID: id, UserID: uid})
	}
	return d, nil
}

func (r *fakeTeamRepo) IsManager(_ context.Context, t, u uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.managers[t][u]
	return ok, nil
}

func (r *fakeTeamRepo) IsMainManager(_ context.Context, t, u uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.managers[t][u], nil
}

func (r *fakeTeamRepo) IsMember(_ context.Context, t, u uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.members[t][u], nil
}

func (r *fakeTeamRepo) AddMember(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members[t] == nil {
		r.members[t] = map[uuid.UUID]bool{}
	}
	if r.members[t][u] {
		return httpx.ErrConflict
	}
	r.members[t][u] = true
	return nil
}

func (r *fakeTeamRepo) RemoveMember(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.members[t][u] {
		return httpx.ErrNotFound
	}
	delete(r.members[t], u)
	return nil
}

func (r *fakeTeamRepo) AddManager(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.managers[t] == nil {
		r.managers[t] = map[uuid.UUID]bool{}
	}
	if _, ok := r.managers[t][u]; ok {
		return httpx.ErrConflict
	}
	r.managers[t][u] = false // not main
	return nil
}

func (r *fakeTeamRepo) RemoveManager(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.managers[t][u]; !ok {
		return httpx.ErrNotFound
	}
	delete(r.managers[t], u)
	return nil
}

func (r *fakeTeamRepo) CountMainManagers(_ context.Context, t uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, main := range r.managers[t] {
		if main {
			n++
		}
	}
	return n, nil
}

func (r *fakeTeamRepo) ManagersOf(_ context.Context, uid uuid.UUID) ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[uuid.UUID]struct{}{}
	for teamID, members := range r.members {
		if _, isMember := members[uid]; !isMember {
			continue
		}
		for mgr := range r.managers[teamID] {
			seen[mgr] = struct{}{}
		}
	}
	out := make([]uuid.UUID, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	return out, nil
}

// fakeAuthClient — controllable.
type fakeAuthClient struct {
	exists  map[uuid.UUID]bool
	failure error
}

func newFakeAuth() *fakeAuthClient { return &fakeAuthClient{exists: map[uuid.UUID]bool{}} }

func (f *fakeAuthClient) UserExists(_ context.Context, uid uuid.UUID) (bool, error) {
	if f.failure != nil {
		return false, f.failure
	}
	return f.exists[uid], nil
}
