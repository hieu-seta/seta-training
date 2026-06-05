// Package service holds the business rules for teams.
// All RBAC + cross-svc checks live here; handlers are dumb.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/authclient"
	"github.com/hieu-seta/seta-training/pkg/cache"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/logger"
	"github.com/hieu-seta/seta-training/services/team/internal/model"
	"github.com/hieu-seta/seta-training/services/team/internal/repo"
	"golang.org/x/sync/singleflight"
)

const teamDetailTTL = 5 * time.Minute

// TeamService orchestrates teams + RBAC.
type TeamService struct {
	repo    repo.TeamRepo
	auth    authclient.AuthClient
	pub     events.Publisher
	cache   cache.Cache
	sf      singleflight.Group
	newUUID func() uuid.UUID
	now     func() time.Time
}

// New builds a TeamService.
func New(r repo.TeamRepo, a authclient.AuthClient, pub events.Publisher) *TeamService {
	if pub == nil {
		pub = events.Noop{}
	}
	return &TeamService{repo: r, auth: a, pub: pub, cache: cache.Noop{}, newUUID: uuid.New, now: time.Now}
}

// WithCache injects a Cache implementation (Noop is the default).
func (s *TeamService) WithCache(c cache.Cache) *TeamService {
	if c == nil {
		c = cache.Noop{}
	}
	s.cache = c
	return s
}

// Create makes a new team w/ caller as main manager.
func (s *TeamService) Create(ctx context.Context, name string, creator uuid.UUID) (*model.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return nil, fmt.Errorf("%w: name must be 1..64", httpx.ErrBadRequest)
	}
	t := &model.Team{ID: s.newUUID(), Name: name, CreatedBy: creator}
	if err := s.repo.Create(ctx, t, creator); err != nil {
		return nil, err
	}
	s.emit(ctx, events.SubjTeamCreated, events.TeamCreated{
		Envelope: s.envelope(ctx, events.SubjTeamCreated),
		TeamID:   t.ID, Name: t.Name, CreatedBy: creator,
	}, events.MsgID(events.SubjTeamCreated, t.ID))
	return t, nil
}

// Detail returns team + members + managers, restricted to team members/mgrs.
// Read-through cache (TeamMembersKey, 5m TTL); invalidator Dels on team.activity events.
func (s *TeamService) Detail(ctx context.Context, teamID, caller uuid.UUID) (*model.TeamDetail, error) {
	allowed, err := s.callerHasAccess(ctx, teamID, caller)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, httpx.ErrForbidden
	}
	key := cache.TeamMembersKey(teamID)
	var hit model.TeamDetail
	if err := s.cache.GetJSON(ctx, key, &hit); err == nil {
		return &hit, nil
	}
	v, err, _ := s.sf.Do("detail:"+teamID.String(), func() (any, error) {
		d, derr := s.repo.Detail(ctx, teamID)
		if derr != nil {
			return nil, derr
		}
		_ = s.cache.SetJSON(ctx, key, d, teamDetailTTL)
		return d, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.TeamDetail), nil
}

// List returns all teams (paged). Stage 1: any authd user.
func (s *TeamService) List(ctx context.Context, limit, offset int) ([]model.Team, error) {
	return s.repo.List(ctx, limit, offset)
}

// AddMember adds target to team. Caller must be a manager of the team and target must exist in auth-svc.
func (s *TeamService) AddMember(ctx context.Context, teamID, caller, target uuid.UUID) error {
	if err := s.assertManager(ctx, teamID, caller); err != nil {
		return err
	}
	if err := s.assertUserExists(ctx, target); err != nil {
		return err
	}
	if err := s.repo.AddMember(ctx, teamID, target); err != nil {
		return err
	}
	s.emit(ctx, events.SubjMemberAdded, events.MemberChanged{
		Envelope: s.envelope(ctx, events.SubjMemberAdded),
		TeamID:   teamID, UserID: target, Actor: caller,
	}, events.MsgID(events.SubjMemberAdded, teamID, target))
	return nil
}

// RemoveMember drops target from team. Caller must be a manager of the team.
func (s *TeamService) RemoveMember(ctx context.Context, teamID, caller, target uuid.UUID) error {
	if err := s.assertManager(ctx, teamID, caller); err != nil {
		return err
	}
	if err := s.repo.RemoveMember(ctx, teamID, target); err != nil {
		return err
	}
	s.emit(ctx, events.SubjMemberRemoved, events.MemberChanged{
		Envelope: s.envelope(ctx, events.SubjMemberRemoved),
		TeamID:   teamID, UserID: target, Actor: caller,
	}, events.MsgID(events.SubjMemberRemoved, teamID, target))
	return nil
}

// AddManager promotes target to manager. Caller must be MAIN manager and target must exist.
func (s *TeamService) AddManager(ctx context.Context, teamID, caller, target uuid.UUID) error {
	if err := s.assertMainManager(ctx, teamID, caller); err != nil {
		return err
	}
	if err := s.assertUserExists(ctx, target); err != nil {
		return err
	}
	if err := s.repo.AddManager(ctx, teamID, target); err != nil {
		return err
	}
	s.emit(ctx, events.SubjManagerAdded, events.MemberChanged{
		Envelope: s.envelope(ctx, events.SubjManagerAdded),
		TeamID:   teamID, UserID: target, Actor: caller,
	}, events.MsgID(events.SubjManagerAdded, teamID, target))
	return nil
}

// ManagersOf returns the manager uids across all teams target is a member of.
func (s *TeamService) ManagersOf(ctx context.Context, target uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.ManagersOf(ctx, target)
}

// RemoveManager demotes target. Caller must be MAIN manager; can't remove the last main manager.
func (s *TeamService) RemoveManager(ctx context.Context, teamID, caller, target uuid.UUID) error {
	if err := s.assertMainManager(ctx, teamID, caller); err != nil {
		return err
	}
	// Disallow self-removal if you're the only main mgr.
	if caller == target {
		n, err := s.repo.CountMainManagers(ctx, teamID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return fmt.Errorf("%w: cannot remove the last main manager", httpx.ErrForbidden)
		}
	}
	_ = errors.New // appease linter
	if err := s.repo.RemoveManager(ctx, teamID, target); err != nil {
		return err
	}
	s.emit(ctx, events.SubjManagerRemoved, events.MemberChanged{
		Envelope: s.envelope(ctx, events.SubjManagerRemoved),
		TeamID:   teamID, UserID: target, Actor: caller,
	}, events.MsgID(events.SubjManagerRemoved, teamID, target))
	return nil
}

// emit publishes an event. Errors are logged but don't fail the business op.
// Publish-after-commit: at most-once delivery is acceptable for stage 3.
func (s *TeamService) emit(ctx context.Context, subj string, payload any, msgID string) {
	_ = s.pub.Publish(ctx, subj, payload, msgID) // intentionally swallow err — caller already committed.
}

// envelope stamps event Type, time, and RequestID (pulled from ctx).
func (s *TeamService) envelope(ctx context.Context, eventType string) events.Envelope {
	return events.Envelope{
		Type:       eventType,
		OccurredAt: s.now(),
		RequestID:  logger.ReqIDFromContext(ctx),
	}
}

func (s *TeamService) assertManager(ctx context.Context, teamID, uid uuid.UUID) error {
	ok, err := s.repo.IsManager(ctx, teamID, uid)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.ErrForbidden
	}
	return nil
}

func (s *TeamService) assertMainManager(ctx context.Context, teamID, uid uuid.UUID) error {
	ok, err := s.repo.IsMainManager(ctx, teamID, uid)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.ErrForbidden
	}
	return nil
}

func (s *TeamService) assertUserExists(ctx context.Context, uid uuid.UUID) error {
	exists, err := s.auth.UserExists(ctx, uid)
	if err != nil {
		if authclient.IsUnavailable(err) {
			return fmt.Errorf("%w: auth-svc unavailable", httpx.ErrUnavailable)
		}
		return err
	}
	if !exists {
		return fmt.Errorf("%w: user not found", httpx.ErrNotFound)
	}
	return nil
}

func (s *TeamService) callerHasAccess(ctx context.Context, teamID, uid uuid.UUID) (bool, error) {
	mgr, err := s.repo.IsManager(ctx, teamID, uid)
	if err != nil {
		return false, err
	}
	if mgr {
		return true, nil
	}
	mem, err := s.repo.IsMember(ctx, teamID, uid)
	if err != nil {
		return false, err
	}
	return mem, nil
}

// EnsureNotInternal is here so the linter doesn't complain about errors.Is below.
var _ = errors.Is
