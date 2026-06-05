package jwtauth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Sentinel errors returned by Parse.
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrBadAlgorithm = errors.New("bad algorithm")
)

// Signer mints + parses HS256 JWTs. Pure — no I/O.
type Signer struct {
	secret []byte
	now    func() time.Time
}

// NewSigner builds a Signer w/ the given HMAC secret + wall clock.
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret), now: time.Now}
}

// WithClock injects a clock for tests.
func (s *Signer) WithClock(now func() time.Time) *Signer {
	s.now = now
	return s
}

// MintAccess creates a short-lived access token. jti optional (empty = none).
func (s *Signer) MintAccess(uid, role string, ttl time.Duration) (string, error) {
	return s.mint(uid, role, "", "", ttl)
}

// MintRefresh creates a refresh token w/ jti + family. Caller stores jti in Redis.
func (s *Signer) MintRefresh(uid, role, jti, family string, ttl time.Duration) (string, error) {
	if jti == "" || family == "" {
		return "", errors.New("refresh requires jti + family")
	}
	return s.mint(uid, role, jti, family, ttl)
}

func (s *Signer) mint(uid, role, jti, family string, ttl time.Duration) (string, error) {
	now := s.now()
	c := Claims{
		UID:    uid,
		Role:   role,
		Family: family,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uid,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(s.secret)
}

// Parse validates HS256 + signature + exp. Returns claims or error.
func (s *Signer) Parse(token string) (*Claims, error) {
	keyFn := func(t *jwt.Token) (any, error) {
		// alg-confusion defense (RFC 8725 §3.1)
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("%w: got %s", ErrBadAlgorithm, t.Method.Alg())
		}
		return s.secret, nil
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	var c Claims
	tok, err := parser.ParseWithClaims(token, &c, keyFn)
	if err != nil {
		return nil, errors.Join(ErrInvalidToken, err)
	}
	if !tok.Valid {
		return nil, ErrInvalidToken
	}
	return &c, nil
}
