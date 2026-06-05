package jwtauth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
)

const testSecret = "test_secret_at_least_32_bytes_______"

func TestSigner_MintAccess_Parse_RoundTrip(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	tok, err := s.MintAccess("u-1", "manager", 5*time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	c, err := s.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.UID != "u-1" || c.Role != "manager" {
		t.Errorf("claims mismatch: %+v", c)
	}
	if c.Subject != "u-1" {
		t.Errorf("sub mismatch: %q", c.Subject)
	}
}

func TestSigner_MintRefresh_RequiresJtiAndFamily(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	cases := []struct {
		name, jti, fam string
	}{
		{"no jti", "", "f"},
		{"no fam", "j", ""},
		{"neither", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.MintRefresh("u", "r", tc.jti, tc.fam, time.Minute); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestSigner_Parse_Expired(t *testing.T) {
	past := time.Now().Add(-2 * time.Hour)
	s := jwtauth.NewSigner(testSecret).WithClock(func() time.Time { return past })
	tok, _ := s.MintAccess("u", "member", 1*time.Minute)
	// clock back to now → already expired
	live := jwtauth.NewSigner(testSecret)
	if _, err := live.Parse(tok); err == nil {
		t.Fatal("expected expired token to fail parse")
	}
}

func TestSigner_Parse_TamperedSig(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	tok, _ := s.MintAccess("u", "member", time.Minute)
	parts := strings.Split(tok, ".")
	parts[2] = "tampered"
	bad := strings.Join(parts, ".")
	if _, err := s.Parse(bad); err == nil {
		t.Fatal("expected tampered sig to fail")
	}
}

func TestSigner_Parse_WrongSecret(t *testing.T) {
	a := jwtauth.NewSigner(testSecret)
	b := jwtauth.NewSigner("other_secret_at_least_32_bytes_____x")
	tok, _ := a.MintAccess("u", "member", time.Minute)
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("expected wrong-secret to fail")
	}
}

// alg-confusion: token claims `none` alg → must reject.
func TestSigner_Parse_NoneAlgRejected(t *testing.T) {
	c := jwtauth.Claims{UID: "evil", Role: "manager"}
	c.Subject = "evil"
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, c)
	raw, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	s := jwtauth.NewSigner(testSecret)
	if _, err := s.Parse(raw); err == nil {
		t.Fatal("expected none-alg to be rejected")
	}
}

// alg-confusion: HS256 token signed w/ wrong method label — reject.
func TestSigner_Parse_HS512Rejected(t *testing.T) {
	c := jwtauth.Claims{UID: "u", Role: "member"}
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	tok := jwt.NewWithClaims(jwt.SigningMethodHS512, c)
	raw, _ := tok.SignedString([]byte(testSecret))
	s := jwtauth.NewSigner(testSecret)
	if _, err := s.Parse(raw); err == nil {
		t.Fatal("expected HS512 to be rejected by HS256-only signer")
	}
}

func TestSigner_RefreshClaimsCarryFamily(t *testing.T) {
	s := jwtauth.NewSigner(testSecret)
	tok, err := s.MintRefresh("u", "member", "jti-1", "fam-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "jti-1" || c.Family != "fam-1" {
		t.Errorf("got jti=%q fam=%q", c.ID, c.Family)
	}
}
