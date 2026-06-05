package events

import (
	"time"

	"github.com/google/uuid"
)

// Envelope is shared by all payloads — keeps consumer parsing uniform.
// RequestID lets the audit-worker correlate an event back to the HTTP request
// that triggered it. Omitted from JSON when empty (back-compat with old events).
type Envelope struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	RequestID  string    `json:"request_id,omitempty"`
}

// TeamCreated — emitted after team-svc creates a team.
type TeamCreated struct {
	Envelope
	TeamID    uuid.UUID `json:"team_id"`
	Name      string    `json:"name"`
	CreatedBy uuid.UUID `json:"created_by"`
}

// MemberChanged covers add/remove for both members and managers.
type MemberChanged struct {
	Envelope
	TeamID uuid.UUID `json:"team_id"`
	UserID uuid.UUID `json:"user_id"`
	Actor  uuid.UUID `json:"actor"`
	IsMain bool      `json:"is_main,omitempty"`
}

// AssetChanged is emitted for folder + note CRUD.
type AssetChanged struct {
	Envelope
	AssetID  uuid.UUID `json:"asset_id"`
	OwnerID  uuid.UUID `json:"owner_id"`
	FolderID uuid.UUID `json:"folder_id,omitempty"` // notes only
	Actor    uuid.UUID `json:"actor"`
}

// ShareChanged is emitted for share/revoke ops.
type ShareChanged struct {
	Envelope
	AssetID uuid.UUID `json:"asset_id"`
	OwnerID uuid.UUID `json:"owner_id"`
	UserID  uuid.UUID `json:"user_id"`
	Access  string    `json:"access,omitempty"` // empty on revoke
	Actor   uuid.UUID `json:"actor"`
}
