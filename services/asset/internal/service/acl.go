// Package service holds asset-svc business logic + ACL resolver.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/teamclient"
	"github.com/hieu-seta/seta-training/services/asset/internal/model"
	"github.com/hieu-seta/seta-training/services/asset/internal/repo"
)

// AssetKind discriminates folder vs note.
type AssetKind string

// Kinds.
const (
	KindFolder AssetKind = "folder"
	KindNote   AssetKind = "note"
)

// Op discriminates read vs write.
type Op string

// Ops.
const (
	OpRead  Op = "read"
	OpWrite Op = "write"
)

// ACL resolves access decisions across 3 sources:
//   - owner
//   - explicit share (incl. folder→note inheritance)
//   - manager-of-owner (read-only)
type ACL struct {
	folders repo.FolderRepo
	notes   repo.NoteRepo
	team    teamclient.TeamClient
}

// NewACL wires the resolver.
func NewACL(f repo.FolderRepo, n repo.NoteRepo, t teamclient.TeamClient) *ACL {
	return &ACL{folders: f, notes: n, team: t}
}

// Check returns nil if uid may perform op on asset, else httpx.ErrForbidden / ErrNotFound / ErrUnavailable.
func (a *ACL) Check(ctx context.Context, uid uuid.UUID, kind AssetKind, assetID uuid.UUID, op Op) error {
	owner, err := a.owner(ctx, kind, assetID)
	if err != nil {
		return err
	}

	// 1. Owner — any op.
	if owner == uid {
		return nil
	}

	// 2. Explicit share.
	access, err := a.shareAccess(ctx, kind, assetID, uid)
	if err != nil {
		return err
	}
	if accessGrants(access, op) {
		return nil
	}

	// 3. Manager oversight (read only).
	if op == OpRead {
		mgrs, err := a.team.ManagersOf(ctx, owner)
		if err != nil {
			return err
		}
		for _, m := range mgrs {
			if m == uid {
				return nil
			}
		}
	}

	return httpx.ErrForbidden
}

func (a *ACL) owner(ctx context.Context, kind AssetKind, id uuid.UUID) (uuid.UUID, error) {
	switch kind {
	case KindFolder:
		f, err := a.folders.ByID(ctx, id)
		if err != nil {
			return uuid.Nil, err
		}
		return f.OwnerID, nil
	case KindNote:
		n, err := a.notes.ByID(ctx, id)
		if err != nil {
			return uuid.Nil, err
		}
		return n.OwnerID, nil
	}
	return uuid.Nil, errors.New("acl: unknown kind")
}

func (a *ACL) shareAccess(ctx context.Context, kind AssetKind, id, uid uuid.UUID) (string, error) {
	switch kind {
	case KindFolder:
		return a.folders.ShareAccess(ctx, id, uid)
	case KindNote:
		// Note ACL = note_share UNION parent folder_share (handled in repo).
		return a.notes.ShareAccess(ctx, id, uid)
	}
	return "", nil
}

func accessGrants(access string, op Op) bool {
	switch op {
	case OpRead:
		return access == model.AccessRead || access == model.AccessWrite
	case OpWrite:
		return access == model.AccessWrite
	}
	return false
}
