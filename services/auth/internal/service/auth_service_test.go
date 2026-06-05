package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/auth/internal/model"
	"github.com/hieu-seta/seta-training/services/auth/internal/service"
)

const testSecret = "auth_test_secret_at_least_32_bytes____"

func newSvc(t *testing.T) (*service.AuthService, *fakeRefreshRepo) {
	t.Helper()
	u := newFakeUserRepo()
	r := newFakeRefreshRepo()
	s := service.New(u, r, jwtauth.NewSigner(testSecret), service.Config{
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
		BcryptCost: 4, // keep tests fast
	})
	return s, r
}

const fixturePassword = "password123"

func mustRegister(t *testing.T, s *service.AuthService, email, role string) *model.User {
	t.Helper()
	u, err := s.Register(context.Background(), "Alice", email, fixturePassword, role)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return u
}

func TestRegister_HappyPath(t *testing.T) {
	s, _ := newSvc(t)
	u := mustRegister(t, s, "alice@example.com", model.RoleMember)
	if u.Email != "alice@example.com" || u.Role != model.RoleMember {
		t.Errorf("unexpected user: %+v", u)
	}
	if u.PasswordHash == "" || u.PasswordHash == "password123" {
		t.Errorf("password not hashed")
	}
}

func TestRegister_Invalid(t *testing.T) {
	s, _ := newSvc(t)
	cases := []struct {
		name, email, pw, role string
	}{
		{"empty email", "", "password123", model.RoleMember},
		{"empty pw", "a@b.c", "", model.RoleMember},
		{"short pw", "a@b.c", "short", model.RoleMember},
		{"bad role", "a@b.c", "password123", "admin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Register(context.Background(), "A", tc.email, tc.pw, tc.role)
			if !errors.Is(err, httpx.ErrBadRequest) {
				t.Errorf("want bad request, got %v", err)
			}
		})
	}
}

func TestRegister_DuplicateEmail_Conflict(t *testing.T) {
	s, _ := newSvc(t)
	mustRegister(t, s, "dup@example.com", model.RoleMember)
	_, err := s.Register(context.Background(), "Other", "dup@example.com", "password123", model.RoleMember)
	if !errors.Is(err, httpx.ErrConflict) {
		t.Errorf("want conflict, got %v", err)
	}
}

func TestLogin_HappyPath(t *testing.T) {
	s, _ := newSvc(t)
	mustRegister(t, s, "bob@example.com", model.RoleManager)
	_, pair, err := s.Login(context.Background(), "BOB@example.com", "password123") // case-insens
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if pair.Access == "" || pair.Refresh == "" {
		t.Errorf("missing tokens: %+v", pair)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	s, _ := newSvc(t)
	mustRegister(t, s, "c@example.com", model.RoleMember)
	_, _, err := s.Login(context.Background(), "c@example.com", "WRONG")
	if !errors.Is(err, httpx.ErrUnauthd) {
		t.Errorf("want unauthd, got %v", err)
	}
}

func TestLogin_UnknownUser_Unauthd(t *testing.T) {
	s, _ := newSvc(t)
	_, _, err := s.Login(context.Background(), "ghost@example.com", "password123")
	if !errors.Is(err, httpx.ErrUnauthd) {
		t.Errorf("want unauthd, got %v", err)
	}
}

func TestRefresh_HappyRotate(t *testing.T) {
	s, _ := newSvc(t)
	mustRegister(t, s, "d@example.com", model.RoleMember)
	_, pair, _ := s.Login(context.Background(), "d@example.com", "password123")
	pair2, err := s.Refresh(context.Background(), pair.Refresh)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if pair2.Refresh == pair.Refresh {
		t.Errorf("refresh token not rotated")
	}
}

// Reusing old refresh after rotation must wipe family + 401.
func TestRefresh_ReplayWipesFamily(t *testing.T) {
	s, rr := newSvc(t)
	mustRegister(t, s, "e@example.com", model.RoleMember)
	_, pair, _ := s.Login(context.Background(), "e@example.com", "password123")
	pair2, err := s.Refresh(context.Background(), pair.Refresh)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	// Replay the original (now-revoked) refresh.
	_, err = s.Refresh(context.Background(), pair.Refresh)
	if !errors.Is(err, httpx.ErrUnauthd) {
		t.Fatalf("expected unauthd on replay, got %v", err)
	}
	// Family must be wiped → pair2 (legit but same family) now invalid too.
	_, err = s.Refresh(context.Background(), pair2.Refresh)
	if !errors.Is(err, httpx.ErrUnauthd) {
		t.Errorf("expected family-wipe to invalidate live token, got %v", err)
	}
	// All keys in family gone.
	claims, _ := jwtauth.NewSigner(testSecret).Parse(pair2.Refresh)
	if rr.familySize(claims.Family) != 0 {
		t.Errorf("family not wiped, size=%d", rr.familySize(claims.Family))
	}
}

func TestRefresh_InvalidToken_Unauthd(t *testing.T) {
	s, _ := newSvc(t)
	_, err := s.Refresh(context.Background(), "not.a.token")
	if !errors.Is(err, httpx.ErrUnauthd) {
		t.Errorf("want unauthd, got %v", err)
	}
}

func TestLogout_DeletesToken(t *testing.T) {
	s, _ := newSvc(t)
	mustRegister(t, s, "f@example.com", model.RoleMember)
	_, pair, _ := s.Login(context.Background(), "f@example.com", "password123")
	if err := s.Logout(context.Background(), pair.Refresh); err != nil {
		t.Fatalf("logout: %v", err)
	}
	// Subsequent refresh should fail.
	_, err := s.Refresh(context.Background(), pair.Refresh)
	if !errors.Is(err, httpx.ErrUnauthd) {
		t.Errorf("want unauthd after logout, got %v", err)
	}
}

func TestUserExists(t *testing.T) {
	s, _ := newSvc(t)
	u := mustRegister(t, s, "g@example.com", model.RoleMember)
	ok, err := s.UserExists(context.Background(), u.ID)
	if err != nil || !ok {
		t.Errorf("expected exists=true, got %v err=%v", ok, err)
	}
}
