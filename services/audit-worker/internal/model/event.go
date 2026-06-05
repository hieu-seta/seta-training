// Package model holds the audit event row type.
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Event is an append-only audit row.
type Event struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Subject    string         `gorm:"type:text;not null"`
	IdemKey    string         `gorm:"type:text;not null;uniqueIndex;column:idem_key"`
	OccurredAt time.Time      `gorm:"not null;column:occurred_at"`
	ActorUID   *uuid.UUID     `gorm:"type:uuid;column:actor_uid"`
	Payload    datatypes.JSON `gorm:"type:jsonb;not null"`
	ReceivedAt time.Time      `gorm:"not null;default:now();column:received_at"`
}

// TableName forces "events".
func (Event) TableName() string { return "events" }
