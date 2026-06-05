package model

import (
	"time"

	"github.com/google/uuid"
)

// Team row.
type Team struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string    `gorm:"type:text;not null"   json:"name"`
	CreatedBy uuid.UUID `gorm:"type:uuid;not null;column:created_by" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName forces "teams".
func (Team) TableName() string { return "teams" }

// TeamMember row in team_members.
type TeamMember struct {
	TeamID  uuid.UUID `gorm:"type:uuid;primaryKey;column:team_id" json:"team_id"`
	UserID  uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id" json:"user_id"`
	AddedAt time.Time `json:"added_at"`
}

// TableName forces "team_members".
func (TeamMember) TableName() string { return "team_members" }

// TeamManager row in team_managers.
type TeamManager struct {
	TeamID  uuid.UUID `gorm:"type:uuid;primaryKey;column:team_id" json:"team_id"`
	UserID  uuid.UUID `gorm:"type:uuid;primaryKey;column:user_id" json:"user_id"`
	IsMain  bool      `gorm:"not null;default:false;column:is_main" json:"is_main"`
	AddedAt time.Time `json:"added_at"`
}

// TableName forces "team_managers".
func (TeamManager) TableName() string { return "team_managers" }

// TeamDetail = aggregated read-model returned by GetByID.
type TeamDetail struct {
	Team
	Managers []TeamManager `json:"managers"`
	Members  []TeamMember  `json:"members"`
}
