package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/cache"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

// fakeCache records Del calls; reads always miss (unused by the invalidator).
type fakeCache struct {
	mu      sync.Mutex
	delKeys [][]string
	delErr  error
}

func (c *fakeCache) GetJSON(_ context.Context, _ string, _ any) error { return cache.ErrMiss }
func (c *fakeCache) SetJSON(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (c *fakeCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delKeys = append(c.delKeys, append([]string(nil), keys...))
	return c.delErr
}
func (c *fakeCache) calls() [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.delKeys
}

// fakeMsg implements jetstream.Msg for the bits handle() touches (Data, Subject, Ack).
type fakeMsg struct {
	data    []byte
	subject string
	acked   bool
}

func (m *fakeMsg) Data() []byte                                { return m.data }
func (m *fakeMsg) Subject() string                             { return m.subject }
func (m *fakeMsg) Ack() error                                  { m.acked = true; return nil }
func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error)   { return &jetstream.MsgMetadata{}, nil }
func (m *fakeMsg) Headers() nats.Header                        { return nil }
func (m *fakeMsg) Reply() string                               { return "" }
func (m *fakeMsg) DoubleAck(context.Context) error             { return nil }
func (m *fakeMsg) Nak() error                                  { return nil }
func (m *fakeMsg) NakWithDelay(time.Duration) error            { return nil }
func (m *fakeMsg) InProgress() error                           { return nil }
func (m *fakeMsg) Term() error                                 { return nil }
func (m *fakeMsg) TermWithReason(string) error                 { return nil }

func newInvalidator(c cache.Cache) *CacheInvalidator {
	return &CacheInvalidator{cache: c, log: zerolog.Nop()}
}

func TestCacheInvalidator_Handle(t *testing.T) {
	teamID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name     string
		subject  string
		payload  string
		wantKeys []string // nil => no Del expected
		acked    bool
	}{
		{
			name:    "team_created (no user) dels members+managers",
			subject: events.SubjTeamCreated,
			payload: `{"team_id":"` + teamID.String() + `"}`,
			wantKeys: []string{
				cache.TeamMembersKey(teamID),
				cache.TeamManagersKey(teamID),
			},
			acked: true,
		},
		{
			name:    "member_added dels members+managers+managers-of",
			subject: events.SubjMemberAdded,
			payload: `{"team_id":"` + teamID.String() + `","user_id":"` + userID.String() + `"}`,
			wantKeys: []string{
				cache.TeamMembersKey(teamID),
				cache.TeamManagersKey(teamID),
				cache.ManagersOfKey(userID),
			},
			acked: true,
		},
		{
			name:    "member_removed dels members+managers+managers-of",
			subject: events.SubjMemberRemoved,
			payload: `{"team_id":"` + teamID.String() + `","user_id":"` + userID.String() + `"}`,
			wantKeys: []string{
				cache.TeamMembersKey(teamID),
				cache.TeamManagersKey(teamID),
				cache.ManagersOfKey(userID),
			},
			acked: true,
		},
		{
			name:    "manager_added dels members+managers+managers-of",
			subject: events.SubjManagerAdded,
			payload: `{"team_id":"` + teamID.String() + `","user_id":"` + userID.String() + `"}`,
			wantKeys: []string{
				cache.TeamMembersKey(teamID),
				cache.TeamManagersKey(teamID),
				cache.ManagersOfKey(userID),
			},
			acked: true,
		},
		{
			name:    "manager_removed dels members+managers+managers-of",
			subject: events.SubjManagerRemoved,
			payload: `{"team_id":"` + teamID.String() + `","user_id":"` + userID.String() + `"}`,
			wantKeys: []string{
				cache.TeamMembersKey(teamID),
				cache.TeamManagersKey(teamID),
				cache.ManagersOfKey(userID),
			},
			acked: true,
		},
		{
			name:     "bad payload: no Del, still acked",
			subject:  events.SubjMemberAdded,
			payload:  `{not json`,
			wantKeys: nil,
			acked:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeCache{}
			inv := newInvalidator(c)
			msg := &fakeMsg{data: []byte(tt.payload), subject: tt.subject}

			inv.handle(msg)

			if msg.acked != tt.acked {
				t.Errorf("acked = %v, want %v", msg.acked, tt.acked)
			}
			calls := c.calls()
			if tt.wantKeys == nil {
				if len(calls) != 0 {
					t.Fatalf("expected no Del, got %v", calls)
				}
				return
			}
			if len(calls) != 1 {
				t.Fatalf("expected 1 Del call, got %d: %v", len(calls), calls)
			}
			if !equalKeys(calls[0], tt.wantKeys) {
				t.Errorf("Del keys = %v, want %v", calls[0], tt.wantKeys)
			}
		})
	}
}

func equalKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
