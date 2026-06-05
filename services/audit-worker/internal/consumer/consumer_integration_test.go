//go:build integration

package consumer_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/consumer"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/model"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/repo"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/service"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/testutil"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

// waitFor polls fn until it returns true or timeout. Used to wait for async
// consumer dispatch + DB writes.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func newSilentLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestConsumer_FullPipeline_RoundsTripToDB(t *testing.T) {
	nc, js := testutil.StartNATS(t)
	db := testutil.StartPG(t)
	ctx := context.Background()

	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	cons := consumer.New(js, service.New(repo.New(db)), newSilentLogger())
	if err := cons.Start(ctx); err != nil {
		t.Fatalf("consumer start: %v", err)
	}
	t.Cleanup(cons.Stop)

	url := nc.ConnectedUrl()
	jsConn, err := events.Connect(ctx, url)
	if err != nil {
		t.Fatalf("events connect: %v", err)
	}
	t.Cleanup(jsConn.Close)
	pub := jsConn.Publisher()

	team := uuid.New()
	creator := uuid.New()
	subj := events.SubjTeamCreated
	msgID := events.MsgID(subj, team)

	err = pub.Publish(ctx, subj, events.TeamCreated{
		Envelope:  events.Envelope{Type: subj, OccurredAt: time.Now(), RequestID: "req-pipeline"},
		TeamID:    team,
		Name:      "Engineering",
		CreatedBy: creator,
	}, msgID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if !waitFor(t, 10*time.Second, func() bool {
		return testutil.CountEventsBySubject(t, db, subj) >= 1
	}) {
		t.Fatalf("audit row never appeared (got %d)", testutil.CountEventsBySubject(t, db, subj))
	}

	var row model.Event
	if err := db.Raw("SELECT * FROM audit.events WHERE subject = ? LIMIT 1", subj).Scan(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.IdemKey != msgID {
		t.Errorf("idem_key = %q, want %q", row.IdemKey, msgID)
	}
}

func TestConsumer_Dedup_SameMsgIDInsertsOnce(t *testing.T) {
	nc, js := testutil.StartNATS(t)
	db := testutil.StartPG(t)
	ctx := context.Background()

	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	cons := consumer.New(js, service.New(repo.New(db)), newSilentLogger())
	if err := cons.Start(ctx); err != nil {
		t.Fatalf("consumer start: %v", err)
	}
	t.Cleanup(cons.Stop)

	jsConn, err := events.Connect(ctx, nc.ConnectedUrl())
	if err != nil {
		t.Fatalf("events connect: %v", err)
	}
	t.Cleanup(jsConn.Close)
	pub := jsConn.Publisher()

	team := uuid.New()
	subj := events.SubjTeamCreated
	msgID := events.MsgID(subj, team)
	payload := events.TeamCreated{
		Envelope:  events.Envelope{Type: subj, OccurredAt: time.Now()},
		TeamID:    team,
		Name:      "Dedup-Test",
		CreatedBy: uuid.New(),
	}

	// Publish same Msg-Id 3 times — NATS dedup (5m window) should drop 2 of them,
	// and the audit_repo's ON CONFLICT DO NOTHING catches anything that slips
	// through (e.g., if the dedup window expired mid-test).
	for i := 0; i < 3; i++ {
		if err := pub.Publish(ctx, subj, payload, msgID); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	// First message should land.
	if !waitFor(t, 10*time.Second, func() bool {
		return testutil.CountEventsBySubject(t, db, subj) >= 1
	}) {
		t.Fatal("first publish never landed")
	}

	// Give the consumer 2s extra to (not) write duplicates.
	time.Sleep(2 * time.Second)

	if got := testutil.CountEventsBySubject(t, db, subj); got != 1 {
		t.Errorf("dedup failed: got %d audit rows, want 1", got)
	}
}

// failingRepo errors the first N inserts then delegates to a real repo.
// Used to validate NAK → JetStream redelivery → eventual success.
type failingRepo struct {
	wrapped     repo.AuditRepo
	failuresLeft int32
}

func (f *failingRepo) Insert(ctx context.Context, e *model.Event) (bool, error) {
	if atomic.LoadInt32(&f.failuresLeft) > 0 {
		atomic.AddInt32(&f.failuresLeft, -1)
		return false, errors.New("synthetic transient failure")
	}
	return f.wrapped.Insert(ctx, e)
}

func TestConsumer_NAK_RedeliverEventuallySucceeds(t *testing.T) {
	nc, js := testutil.StartNATS(t)
	db := testutil.StartPG(t)
	ctx := context.Background()

	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	fr := &failingRepo{wrapped: repo.New(db), failuresLeft: 2}
	cons := consumer.New(js, service.New(fr), newSilentLogger())
	if err := cons.Start(ctx); err != nil {
		t.Fatalf("consumer start: %v", err)
	}
	t.Cleanup(cons.Stop)

	jsConn, err := events.Connect(ctx, nc.ConnectedUrl())
	if err != nil {
		t.Fatalf("events connect: %v", err)
	}
	t.Cleanup(jsConn.Close)

	team := uuid.New()
	subj := events.SubjTeamCreated
	if err := jsConn.Publisher().Publish(ctx, subj, events.TeamCreated{
		Envelope:  events.Envelope{Type: subj, OccurredAt: time.Now()},
		TeamID:    team,
		Name:      "Nak-Retry",
		CreatedBy: uuid.New(),
	}, events.MsgID(subj, team)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// MaxDeliver=5; failuresLeft=2 → 3rd delivery should succeed.
	if !waitFor(t, 30*time.Second, func() bool {
		return testutil.CountEventsBySubject(t, db, subj) >= 1
	}) {
		t.Fatalf("expected row to land after NAK retries, still missing (failuresLeft=%d)",
			atomic.LoadInt32(&fr.failuresLeft))
	}
	if got := atomic.LoadInt32(&fr.failuresLeft); got != 0 {
		t.Errorf("expected all 2 forced failures consumed, got failuresLeft=%d", got)
	}
}

func TestConsumer_GracefulShutdown_DrainsWithinTimeout(t *testing.T) {
	_, js := testutil.StartNATS(t)
	db := testutil.StartPG(t)
	ctx := context.Background()

	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}
	cons := consumer.New(js, service.New(repo.New(db)), newSilentLogger())
	if err := cons.Start(ctx); err != nil {
		t.Fatalf("consumer start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		cons.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned cleanly.
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return within 5s")
	}
}

// TestConsumer_FilterByStream verifies activity and assets streams each route
// to their own durable consumer rather than crossing wires.
func TestConsumer_FilterByStream(t *testing.T) {
	nc, js := testutil.StartNATS(t)
	db := testutil.StartPG(t)
	ctx := context.Background()

	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}
	cons := consumer.New(js, service.New(repo.New(db)), newSilentLogger())
	if err := cons.Start(ctx); err != nil {
		t.Fatalf("consumer start: %v", err)
	}
	t.Cleanup(cons.Stop)

	jsConn, err := events.Connect(ctx, nc.ConnectedUrl())
	if err != nil {
		t.Fatalf("events connect: %v", err)
	}
	t.Cleanup(jsConn.Close)
	pub := jsConn.Publisher()

	teamID := uuid.New()
	assetID := uuid.New()

	if err := pub.Publish(ctx, events.SubjTeamCreated, events.TeamCreated{
		Envelope: events.Envelope{Type: events.SubjTeamCreated, OccurredAt: time.Now()},
		TeamID:   teamID, Name: "Filter-T", CreatedBy: uuid.New(),
	}, events.MsgID(events.SubjTeamCreated, teamID)); err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(ctx, events.SubjFolderCreated, events.AssetChanged{
		Envelope: events.Envelope{Type: events.SubjFolderCreated, OccurredAt: time.Now()},
		AssetID:  assetID, OwnerID: uuid.New(), Actor: uuid.New(),
	}, events.MsgID(events.SubjFolderCreated, assetID)); err != nil {
		t.Fatal(err)
	}

	if !waitFor(t, 10*time.Second, func() bool {
		return testutil.CountEvents(t, db) >= 2
	}) {
		t.Fatalf("expected 2 rows (one per stream), got %d", testutil.CountEvents(t, db))
	}
	if got := testutil.CountEventsBySubject(t, db, events.SubjTeamCreated); got != 1 {
		t.Errorf("team_created rows = %d, want 1", got)
	}
	if got := testutil.CountEventsBySubject(t, db, events.SubjFolderCreated); got != 1 {
		t.Errorf("folder_created rows = %d, want 1", got)
	}
}

// Unused safety net so unused imports don't tank the build during refactors.
var _ = jetstream.AckExplicitPolicy
