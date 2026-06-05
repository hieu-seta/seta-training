package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/auth/internal/handler"
	"github.com/hieu-seta/seta-training/services/auth/internal/model"
	"github.com/hieu-seta/seta-training/services/auth/internal/service"
)

const testSecret = "handler_secret_at_least_32_bytes_______"

// Reuse the fakes idea but redeclared locally — handler pkg is separate from service pkg.

type fUR struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]*model.User
	byEml map[string]*model.User
}

func newFUR() *fUR {
	return &fUR{byID: map[uuid.UUID]*model.User{}, byEml: map[string]*model.User{}}
}
func (r *fUR) Create(_ context.Context, u *model.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byEml[u.Email]; ok {
		return httpx.ErrConflict
	}
	cp := *u
	r.byID[cp.ID] = &cp
	r.byEml[cp.Email] = &cp
	return nil
}
func (r *fUR) ByEmail(_ context.Context, e string) (*model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byEml[e]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *u
	return &cp, nil
}
func (r *fUR) ByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *u
	return &cp, nil
}
func (r *fUR) List(_ context.Context, _, _ int) ([]model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, *u)
	}
	return out, nil
}
func (r *fUR) Exists(_ context.Context, id uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.byID[id]
	return ok, nil
}

type fRR struct {
	mu       sync.Mutex
	tokens   map[string]struct{ uid, family string }
	families map[string]map[string]struct{}
}

func newFRR() *fRR {
	return &fRR{tokens: map[string]struct{ uid, family string }{}, families: map[string]map[string]struct{}{}}
}
func (r *fRR) Store(_ context.Context, jti, uid, family string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[jti] = struct{ uid, family string }{uid, family}
	if r.families[family] == nil {
		r.families[family] = map[string]struct{}{}
	}
	r.families[family][jti] = struct{}{}
	return nil
}
func (r *fRR) Lookup(_ context.Context, jti string) (uid, family string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[jti]
	if !ok {
		return "", "", httpx.ErrUnauthd
	}
	return t.uid, t.family, nil
}
func (r *fRR) Delete(_ context.Context, jti string) error {
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
func (r *fRR) DeleteFamily(_ context.Context, family string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for jti := range r.families[family] {
		delete(r.tokens, jti)
	}
	delete(r.families, family)
	return nil
}

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	users := newFUR()
	refresh := newFRR()
	signer := jwtauth.NewSigner(testSecret)
	svc := service.New(users, refresh, signer, service.Config{
		AccessTTL: time.Minute, RefreshTTL: time.Hour, BcryptCost: 4,
	})
	r := gin.New()
	handler.New(svc, signer).Register(r)
	return r
}

func do(t *testing.T, r http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var b *bytes.Reader
	if body != "" {
		b = bytes.NewReader([]byte(body))
	} else {
		b = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, path, b)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPostUsers_Created(t *testing.T) {
	r := newRouter(t)
	w := do(t, r, http.MethodPost, "/users", `{"username":"alice","email":"alice@example.com","password":"password123","role":"member"}`, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"email":"alice@example.com"`) {
		t.Errorf("body missing email: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password") {
		t.Errorf("password leaked in body: %s", w.Body.String())
	}
}

func TestPostUsers_BadEmail_400(t *testing.T) {
	r := newRouter(t)
	w := do(t, r, http.MethodPost, "/users", `{"username":"x","email":"not-email","password":"password123","role":"member"}`, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPostUsers_DuplicateEmail_409(t *testing.T) {
	r := newRouter(t)
	body := `{"username":"x","email":"d@e.f","password":"password123","role":"member"}`
	if w := do(t, r, http.MethodPost, "/users", body, ""); w.Code != http.StatusCreated {
		t.Fatalf("first create: %d", w.Code)
	}
	w := do(t, r, http.MethodPost, "/users", body, "")
	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
}

func TestLogin_Roundtrip(t *testing.T) {
	r := newRouter(t)
	do(t, r, http.MethodPost, "/users", `{"username":"a","email":"l@e.f","password":"password123","role":"manager"}`, "")
	w := do(t, r, http.MethodPost, "/login", `{"email":"l@e.f","password":"password123"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["access"] == "" || resp["refresh"] == "" {
		t.Errorf("missing tokens in resp: %+v", resp)
	}
}

func TestLogin_WrongPw_401(t *testing.T) {
	r := newRouter(t)
	do(t, r, http.MethodPost, "/users", `{"username":"a","email":"w@e.f","password":"password123","role":"member"}`, "")
	w := do(t, r, http.MethodPost, "/login", `{"email":"w@e.f","password":"WRONG"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestGetUsers_NoJWT_401(t *testing.T) {
	r := newRouter(t)
	w := do(t, r, http.MethodGet, "/users", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestGetUsers_WithJWT_200(t *testing.T) {
	r := newRouter(t)
	do(t, r, http.MethodPost, "/users", `{"username":"a","email":"u@e.f","password":"password123","role":"manager"}`, "")
	loginW := do(t, r, http.MethodPost, "/login", `{"email":"u@e.f","password":"password123"}`, "")
	var lp map[string]string
	_ = json.Unmarshal(loginW.Body.Bytes(), &lp)
	w := do(t, r, http.MethodGet, "/users", "", lp["access"])
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"users"`) {
		t.Errorf("expected users array: %s", w.Body.String())
	}
}

func TestRefresh_Rotates(t *testing.T) {
	r := newRouter(t)
	do(t, r, http.MethodPost, "/users", `{"username":"a","email":"r@e.f","password":"password123","role":"member"}`, "")
	loginW := do(t, r, http.MethodPost, "/login", `{"email":"r@e.f","password":"password123"}`, "")
	var lp map[string]string
	_ = json.Unmarshal(loginW.Body.Bytes(), &lp)
	w := do(t, r, http.MethodPost, "/refresh", `{"refresh":"`+lp["refresh"]+`"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var rp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &rp)
	if rp["refresh"] == lp["refresh"] {
		t.Errorf("refresh token not rotated")
	}
	// Replay original → 401.
	w2 := do(t, r, http.MethodPost, "/refresh", `{"refresh":"`+lp["refresh"]+`"}`, "")
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("replay want 401, got %d", w2.Code)
	}
}

func TestExists_FoundAndMissing(t *testing.T) {
	r := newRouter(t)
	createResp := do(t, r, http.MethodPost, "/users", `{"username":"a","email":"x@e.f","password":"password123","role":"member"}`, "")
	var u map[string]any
	_ = json.Unmarshal(createResp.Body.Bytes(), &u)
	uid := u["id"].(string)

	// /exists is inter-svc → no JWT required.
	w := do(t, r, http.MethodGet, "/users/"+uid+"/exists", "", "")
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	w2 := do(t, r, http.MethodGet, "/users/"+uuid.NewString()+"/exists", "", "")
	if w2.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w2.Code)
	}
}

func TestLogout_Then_RefreshFails(t *testing.T) {
	r := newRouter(t)
	do(t, r, http.MethodPost, "/users", `{"username":"a","email":"lo@e.f","password":"password123","role":"member"}`, "")
	loginW := do(t, r, http.MethodPost, "/login", `{"email":"lo@e.f","password":"password123"}`, "")
	var lp map[string]string
	_ = json.Unmarshal(loginW.Body.Bytes(), &lp)
	w := do(t, r, http.MethodPost, "/logout", `{"refresh":"`+lp["refresh"]+`"}`, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	w2 := do(t, r, http.MethodPost, "/refresh", `{"refresh":"`+lp["refresh"]+`"}`, "")
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("want 401 after logout, got %d", w2.Code)
	}
}
