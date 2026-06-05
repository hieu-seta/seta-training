//go:build integration

package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	natsmod "github.com/testcontainers/testcontainers-go/modules/nats"
)

func startNATS(t *testing.T) (*nats.Conn, jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := []testcontainers.ContainerCustomizer{
		testcontainers.WithCmd("-js", "-DV"),
	}
	if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") == "true" {
		opts = append(opts, testcontainers.WithReuseByName("seta-it-nats-events"))
	}
	c, err := natsmod.Run(ctx, "nats:2.10-alpine", opts...)
	if err != nil {
		t.Fatalf("nats container: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") != "true" {
			_ = c.Terminate(context.Background())
		}
	})
	url, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	t.Cleanup(func() { _ = nc.Drain() })
	return nc, js
}

func TestEnsureStreams_Idempotent(t *testing.T) {
	_, js := startNATS(t)
	ctx := context.Background()
	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("second ensure (idempotent): %v", err)
	}
	// Both streams must exist.
	for _, name := range []string{events.StreamActivity, events.StreamAssets} {
		if _, err := js.Stream(ctx, name); err != nil {
			t.Errorf("stream %s missing: %v", name, err)
		}
	}
}

func TestPublisher_DedupViaMsgID(t *testing.T) {
	_, js := startNATS(t)
	ctx := context.Background()
	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatal(err)
	}
	pub := (&events.JS{}) // sentinel — replace via real connect
	_ = pub
	// Use the package-internal builder directly via the JS facade.
	// Re-use connection by building JS via Connect on the same NATS URL.
	url := js.Conn().ConnectedUrl()
	conn, err := events.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)

	teamID := uuid.New()
	uid := uuid.New()
	actor := uuid.New()
	msgID := events.MsgID(events.SubjMemberAdded, teamID, uid)
	payload := events.MemberChanged{
		Envelope: events.Envelope{Type: events.SubjMemberAdded, OccurredAt: time.Now()},
		TeamID:   teamID, UserID: uid, Actor: actor,
	}
	for i := 0; i < 3; i++ {
		if err := conn.Publisher().Publish(ctx, events.SubjMemberAdded, payload, msgID); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	// Verify only 1 msg stored.
	s, err := js.Stream(ctx, events.StreamActivity)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Errorf("expected 1 msg (deduped), got %d", info.State.Msgs)
	}
}

func TestPublisher_RoundTrip(t *testing.T) {
	_, js := startNATS(t)
	ctx := context.Background()
	if err := events.EnsureStreams(ctx, js); err != nil {
		t.Fatal(err)
	}
	url := js.Conn().ConnectedUrl()
	conn, err := events.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)

	// Ephemeral consumer.
	s, err := js.Stream(ctx, events.StreamActivity)
	if err != nil {
		t.Fatal(err)
	}
	cons, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: events.SubjTeamCreated,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}

	team := uuid.New()
	creator := uuid.New()
	payload := events.TeamCreated{
		Envelope:  events.Envelope{Type: events.SubjTeamCreated, OccurredAt: time.Now()},
		TeamID:    team, Name: "Eng", CreatedBy: creator,
	}
	if err := conn.Publisher().Publish(ctx, events.SubjTeamCreated, payload, events.MsgID(events.SubjTeamCreated, team)); err != nil {
		t.Fatal(err)
	}

	msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	msg, err := cons.Next(jetstream.FetchMaxWait(3 * time.Second))
	if err != nil {
		t.Fatalf("fetch: %v ctxErr=%v", err, msgCtx.Err())
	}
	var got events.TeamCreated
	if err := json.Unmarshal(msg.Data(), &got); err != nil {
		t.Fatal(err)
	}
	if got.TeamID != team || got.Name != "Eng" {
		t.Errorf("got %+v want team=%s name=Eng", got, team)
	}
	if err := msg.Ack(); err != nil {
		t.Errorf("ack: %v", err)
	}
}

func TestConnect_BadURL(t *testing.T) {
	_, err := events.Connect(context.Background(), "nats://nowhere.invalid:4222")
	if err == nil {
		t.Error("expected error on bad URL")
	}
	if errors.Is(err, context.Canceled) {
		t.Error("should not be ctx canceled")
	}
}
