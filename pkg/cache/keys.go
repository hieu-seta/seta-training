package cache

import "github.com/google/uuid"

// DefaultTTL is the cache safety net (5m) — events do the real invalidation; this catches drops.
const DefaultTTL = "5m"

// TeamMembersKey returns the key for the team-members read-model.
func TeamMembersKey(teamID uuid.UUID) string {
	return "team:" + teamID.String() + ":members"
}

// TeamManagersKey returns the key for the team-managers read-model.
func TeamManagersKey(teamID uuid.UUID) string {
	return "team:" + teamID.String() + ":managers"
}

// FolderMetaKey returns the key for folder metadata.
func FolderMetaKey(folderID uuid.UUID) string {
	return "asset:folder:" + folderID.String()
}

// NoteMetaKey returns the key for note metadata.
func NoteMetaKey(noteID uuid.UUID) string {
	return "asset:note:" + noteID.String()
}

// ACLKey returns the key for a cached ACL decision.
// kind: "folder" | "note". op: "read" | "write".
func ACLKey(uid uuid.UUID, kind, assetID, op string) string {
	return "acl:" + uid.String() + ":" + kind + ":" + assetID + ":" + op
}

// ManagersOfKey returns the key for the (uid → managers) lookup used by asset-svc ACL.
func ManagersOfKey(uid uuid.UUID) string {
	return "team:managers-of:" + uid.String()
}
