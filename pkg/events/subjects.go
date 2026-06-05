// Package events defines the wire format for cross-svc messages on NATS JetStream.
package events

// Team activity subjects — emitted by team-svc on state changes.
const (
	StreamActivity = "ACTIVITY"

	SubjTeamCreated     = "team.activity.team_created"
	SubjMemberAdded     = "team.activity.member_added"
	SubjMemberRemoved   = "team.activity.member_removed"
	SubjManagerAdded    = "team.activity.manager_added"
	SubjManagerRemoved  = "team.activity.manager_removed"
	SubjTeamActivityAll = "team.activity.>"
)

// Asset changes subjects — emitted by asset-svc.
const (
	StreamAssets = "ASSETS"

	SubjFolderCreated = "asset.changes.folder_created"
	SubjFolderUpdated = "asset.changes.folder_updated"
	SubjFolderDeleted = "asset.changes.folder_deleted"
	SubjFolderShared  = "asset.changes.folder_shared"
	SubjFolderRevoked = "asset.changes.folder_revoked"
	SubjNoteCreated   = "asset.changes.note_created"
	SubjNoteUpdated   = "asset.changes.note_updated"
	SubjNoteDeleted   = "asset.changes.note_deleted"
	SubjNoteShared    = "asset.changes.note_shared"
	SubjNoteRevoked   = "asset.changes.note_revoked"
	SubjAssetAll      = "asset.changes.>"
)
