package model

import (
	"time"

	"github.com/google/uuid"
)

// Role values.
const (
	RoleManager = "manager"
	RoleMember  = "member"
)

// User row in auth.users.
// PasswordHash never serialized over JSON.
type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Username     string    `gorm:"type:text;not null"  json:"username"`
	Email        string    `gorm:"type:text;not null;uniqueIndex" json:"email"`
	PasswordHash string    `gorm:"type:text;not null"  json:"-"`
	Role         string    `gorm:"type:text;not null"  json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName forces "users" (svc connects w/ search_path=auth).
func (User) TableName() string { return "users" }

// IsValidRole returns true if r is a known role.
func IsValidRole(r string) bool { return r == RoleManager || r == RoleMember }
