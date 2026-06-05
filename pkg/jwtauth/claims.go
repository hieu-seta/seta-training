package jwtauth

import "github.com/golang-jwt/jwt/v5"

// Claims is the JWT payload shape. UID + Role drive ctx; Family carries refresh lineage.
type Claims struct {
	UID    string `json:"uid"`
	Role   string `json:"role"`
	Family string `json:"fam,omitempty"` // refresh-token family for theft detection
	jwt.RegisteredClaims
}
