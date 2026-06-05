// Package model holds asset-svc domain types.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Access values.
const (
	AccessRead  = "read"
	AccessWrite = "write"
)

// IsValidAccess returns true if a is "read" or "write".
func IsValidAccess(a string) bool { return a == AccessRead || a == AccessWrite }

// Folder row.
type Folder struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"        json:"id"`
	OwnerID   uuid.UUID `gorm:"type:uuid;not null;column:owner_id" json:"owner_id"`
	Name      string    `gorm:"type:text;not null"          json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName forces "folders".
func (Folder) TableName() string { return "folders" }

// Note row.
type Note struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"        json:"id"`
	FolderID  uuid.UUID `gorm:"type:uuid;not null;column:folder_id" json:"folder_id"`
	OwnerID   uuid.UUID `gorm:"type:uuid;not null;column:owner_id"  json:"owner_id"`
	Title     string    `gorm:"type:text;not null"          json:"title"`
	Body      string    `gorm:"type:text;not null;default:''" json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName forces "notes".
func (Note) TableName() string { return "notes" }

// FolderShare grants a user access to a folder.
type FolderShare struct {
	FolderID  uuid.UUID `gorm:"type:uuid;primaryKey;column:folder_id" json:"folder_id"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id"   json:"user_id"`
	Access    string    `gorm:"type:text;not null"           json:"access"`
	GrantedAt time.Time `json:"granted_at"`
}

// TableName forces "folder_shares".
func (FolderShare) TableName() string { return "folder_shares" }

// NoteShare grants a user access to a single note.
type NoteShare struct {
	NoteID    uuid.UUID `gorm:"type:uuid;primaryKey;column:note_id" json:"note_id"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id" json:"user_id"`
	Access    string    `gorm:"type:text;not null"           json:"access"`
	GrantedAt time.Time `json:"granted_at"`
}

// TableName forces "note_shares".
func (NoteShare) TableName() string { return "note_shares" }
