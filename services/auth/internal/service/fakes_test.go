package service_test

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/auth/internal/model"
)

// fakeUserRepo — in-memory; deterministic, no SQL.
type fakeUserRepo struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]*model.User
	byEml map[string]*model.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uuid.UUID]*model.User{}, byEml: map[string]*model.User{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *model.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byEml[u.Email]; ok {
		return httpx.ErrConflict
	}
	cp := *u
	cp.CreatedAt, cp.UpdatedAt = time.Now(), time.Now()
	r.byID[cp.ID] = &cp
	r.byEml[cp.Email] = &cp
	return nil
}

func (r *fakeUserRepo) ByEmail(_ context.Context, email string) (*model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byEml[email]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *fakeUserRepo) ByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *fakeUserRepo) List(_ context.Context, limit, _ int) ([]model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, *u)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeUserRepo) Exists(_ context.Context, id uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.byID[id]
	return ok, nil
}

// fakeRefreshRepo — in-memory.
type fakeRefreshRepo struct {
	mu        sync.Mutex
	tokens    map[string]struct{ uid, family string }
	families  map[string]map[string]struct{}
	failStore bool
}

func newFakeRefreshRepo() *fakeRefreshRepo {
	return &fakeRefreshRepo{
		tokens:   map[string]struct{ uid, family string }{},
		families: map[string]map[string]struct{}{},
	}
}

func (r *fakeRefreshRepo) Store(_ context.Context, jti, uid, family string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failStore {
		return httpx.ErrInternal
	}
	r.tokens[jti] = struct{ uid, family string }{uid, family}
	if r.families[family] == nil {
		r.families[family] = map[string]struct{}{}
	}
	r.families[family][jti] = struct{}{}
	return nil
}

func (r *fakeRefreshRepo) Lookup(_ context.Context, jti string) (uid, family string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[jti]
	if !ok {
		return "", "", httpx.ErrUnauthd
	}
	return t.uid, t.family, nil
}

func (r *fakeRefreshRepo) Delete(_ context.Context, jti string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tokens[jti]; ok {
		delete(r.tokens, jti)
		if fam := r.families[t.family]; fam != nil {
			delete(fam, jti)
		}
	}
	return nil
}

func (r *fakeRefreshRepo) DeleteFamily(_ context.Context, family string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for jti := range r.families[family] {
		delete(r.tokens, jti)
	}
	delete(r.families, family)
	return nil
}

func (r *fakeRefreshRepo) familySize(family string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.families[family])
}
