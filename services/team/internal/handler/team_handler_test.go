package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/authclient"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/team/internal/handler"
	"github.com/hieu-seta/seta-training/services/team/internal/model"
	"github.com/hieu-seta/seta-training/services/team/internal/repo"
	"github.com/hieu-seta/seta-training/services/team/internal/service"
)

const testSecret = "team_handler_secret_at_least_32_bytes__"

// in-memory repo (local impl; mirrors service-pkg fake)
type memRepo struct {
	mu       sync.Mutex
	teams    map[uuid.UUID]*model.Team
	members  map[uuid.UUID]map[uuid.UUID]bool
	managers map[uuid.UUID]map[uuid.UUID]bool
}

func newMemRepo() *memRepo {
	return &memRepo{teams: map[uuid.UUID]*model.Team{}, members: map[uuid.UUID]map[uuid.UUID]bool{}, managers: map[uuid.UUID]map[uuid.UUID]bool{}}
}

func (r *memRepo) Create(_ context.Context, t *model.Team, main uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams[t.ID] = t
	if r.managers[t.ID] == nil {
		r.managers[t.ID] = map[uuid.UUID]bool{}
	}
	r.managers[t.ID][main] = true
	return nil
}
func (r *memRepo) ByID(_ context.Context, id uuid.UUID) (*model.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.teams[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *t
	return &cp, nil
}
func (r *memRepo) List(_ context.Context, _, _ int) ([]model.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.Team, 0, len(r.teams))
	for _, t := range r.teams {
		out = append(out, *t)
	}
	return out, nil
}
func (r *memRepo) Detail(_ context.Context, id uuid.UUID) (*model.TeamDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.teams[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	d := &model.TeamDetail{Team: *t}
	for u, m := range r.managers[id] {
		d.Managers = append(d.Managers, model.TeamManager{TeamID: id, UserID: u, IsMain: m})
	}
	for u := range r.members[id] {
		d.Members = append(d.Members, model.TeamMember{TeamID: id, UserID: u})
	}
	return d, nil
}
func (r *memRepo) IsManager(_ context.Context, t, u uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.managers[t][u]
	return ok, nil
}
func (r *memRepo) IsMainManager(_ context.Context, t, u uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.managers[t][u], nil
}
func (r *memRepo) IsMember(_ context.Context, t, u uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.members[t][u], nil
}
func (r *memRepo) AddMember(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members[t] == nil {
		r.members[t] = map[uuid.UUID]bool{}
	}
	r.members[t][u] = true
	return nil
}
func (r *memRepo) RemoveMember(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.members[t], u)
	return nil
}
func (r *memRepo) AddManager(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.managers[t] == nil {
		r.managers[t] = map[uuid.UUID]bool{}
	}
	r.managers[t][u] = false
	return nil
}
func (r *memRepo) RemoveManager(_ context.Context, t, u uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.managers[t], u)
	return nil
}
func (r *memRepo) CountMainManagers(_ context.Context, t uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, m := range r.managers[t] {
		if m {
			n++
		}
	}
	return n, nil
}

func (r *memRepo) ManagersOf(_ context.Context, uid uuid.UUID) ([]uuid.UUID, error) {
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

var _ repo.TeamRepo = (*memRepo)(nil) // compile-time check

func newRouter(t *testing.T, authStub authclient.AuthClient) (*gin.Engine, *memRepo, *jwtauth.Signer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := newMemRepo()
	signer := jwtauth.NewSigner(testSecret)
	svc := service.New(r, authStub, nil)
	g := gin.New()
	handler.New(svc, signer).Register(g)
	return g, r, signer
}

func bearer(s *jwtauth.Signer, uid uuid.UUID, role string) string {
	tok, _ := s.MintAccess(uid.String(), role, time.Minute)
	return tok
}

func do(t *testing.T, r http.Handler, method, path, body, tok string) *httptest.ResponseRecorder {
	t.Helper()
	var b *bytes.Reader
	if body != "" {
		b = bytes.NewReader([]byte(body))
	} else {
		b = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, path, b)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// stub authclient: always exists
type alwaysExists struct{}

func (alwaysExists) UserExists(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }

func TestCreateTeam_ManagerJWT_201(t *testing.T) {
	g, _, s := newRouter(t, alwaysExists{})
	tok := bearer(s, uuid.New(), "manager")
	w := do(t, g, http.MethodPost, "/teams", `{"name":"Eng"}`, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateTeam_MemberJWT_403(t *testing.T) {
	g, _, s := newRouter(t, alwaysExists{})
	tok := bearer(s, uuid.New(), "member")
	w := do(t, g, http.MethodPost, "/teams", `{"name":"Eng"}`, tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

func TestCreateTeam_NoJWT_401(t *testing.T) {
	g, _, _ := newRouter(t, alwaysExists{})
	w := do(t, g, http.MethodPost, "/teams", `{"name":"Eng"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAddMember_AsManager_204(t *testing.T) {
	g, _, s := newRouter(t, alwaysExists{})
	mgr := uuid.New()
	tok := bearer(s, mgr, "manager")
	createW := do(t, g, http.MethodPost, "/teams", `{"name":"X"}`, tok)
	var tm model.Team
	_ = json.Unmarshal(createW.Body.Bytes(), &tm)
	target := uuid.New()
	w := do(t, g, http.MethodPost, "/teams/"+tm.ID.String()+"/members", `{"user_id":"`+target.String()+`"}`, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// stub: returns false → 404 user
type alwaysMissing struct{}

func (alwaysMissing) UserExists(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil }

func TestAddMember_UserMissingInAuth_404(t *testing.T) {
	g, _, s := newRouter(t, alwaysMissing{})
	mgr := uuid.New()
	tok := bearer(s, mgr, "manager")
	createW := do(t, g, http.MethodPost, "/teams", `{"name":"X"}`, tok)
	var tm model.Team
	_ = json.Unmarshal(createW.Body.Bytes(), &tm)
	w := do(t, g, http.MethodPost, "/teams/"+tm.ID.String()+"/members", `{"user_id":"`+uuid.NewString()+`"}`, tok)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// stub: returns httpx.ErrUnavailable
type authDown struct{}

func (authDown) UserExists(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, httpx.ErrUnavailable
}

func TestAddMember_AuthDown_503(t *testing.T) {
	g, _, s := newRouter(t, authDown{})
	mgr := uuid.New()
	tok := bearer(s, mgr, "manager")
	createW := do(t, g, http.MethodPost, "/teams", `{"name":"X"}`, tok)
	var tm model.Team
	_ = json.Unmarshal(createW.Body.Bytes(), &tm)
	w := do(t, g, http.MethodPost, "/teams/"+tm.ID.String()+"/members", `{"user_id":"`+uuid.NewString()+`"}`, tok)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAddManager_NonMainMgr_403(t *testing.T) {
	g, r, s := newRouter(t, alwaysExists{})
	main := uuid.New()
	tokMain := bearer(s, main, "manager")
	createW := do(t, g, http.MethodPost, "/teams", `{"name":"X"}`, tokMain)
	var tm model.Team
	_ = json.Unmarshal(createW.Body.Bytes(), &tm)
	other := uuid.New()
	_ = r.AddManager(context.Background(), tm.ID, other) // non-main
	tokOther := bearer(s, other, "manager")
	w := do(t, g, http.MethodPost, "/teams/"+tm.ID.String()+"/managers", `{"user_id":"`+uuid.NewString()+`"}`, tokOther)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDetail_NonMember_403(t *testing.T) {
	g, _, s := newRouter(t, alwaysExists{})
	main := uuid.New()
	tokMain := bearer(s, main, "manager")
	createW := do(t, g, http.MethodPost, "/teams", `{"name":"X"}`, tokMain)
	var tm model.Team
	_ = json.Unmarshal(createW.Body.Bytes(), &tm)
	outsider := uuid.New()
	tokOut := bearer(s, outsider, "manager")
	w := do(t, g, http.MethodGet, "/teams/"+tm.ID.String(), "", tokOut)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

func TestList_Authd_200(t *testing.T) {
	g, _, s := newRouter(t, alwaysExists{})
	tok := bearer(s, uuid.New(), "member")
	w := do(t, g, http.MethodGet, "/teams", "", tok)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestList_NoJWT_401(t *testing.T) {
	g, _, _ := newRouter(t, alwaysExists{})
	w := do(t, g, http.MethodGet, "/teams", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}
