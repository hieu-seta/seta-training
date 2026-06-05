package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hieu-seta/seta-training/services/audit-worker/internal/model"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/service"
)

type fakeRepo struct {
	rows    map[string]int
	failErr error
	calls   int
}

func newFake() *fakeRepo { return &fakeRepo{rows: map[string]int{}} }

func (r *fakeRepo) Insert(_ context.Context, e *model.Event) (bool, error) {
	r.calls++
	if r.failErr != nil {
		return false, r.failErr
	}
	if _, dup := r.rows[e.IdemKey]; dup {
		return false, nil
	}
	r.rows[e.IdemKey] = 1
	return true, nil
}

func TestRecord_Happy_InsertedTrue(t *testing.T) {
	r := newFake()
	s := service.New(r)
	inserted, err := s.Record(context.Background(), "team.activity.team_created", "id-1", []byte(`{}`))
	if err != nil || !inserted {
		t.Errorf("got inserted=%v err=%v", inserted, err)
	}
}

func TestRecord_Dedup_InsertedFalse_NoErr(t *testing.T) {
	r := newFake()
	s := service.New(r)
	_, _ = s.Record(context.Background(), "team.activity.team_created", "id-2", []byte(`{}`))
	inserted, err := s.Record(context.Background(), "team.activity.team_created", "id-2", []byte(`{}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if inserted {
		t.Errorf("expected dedup (inserted=false)")
	}
}

func TestRecord_FallbackID_When_MsgIDEmpty(t *testing.T) {
	r := newFake()
	s := service.New(r)
	in1, _ := s.Record(context.Background(), "team.activity.team_created", "", []byte(`{"x":1}`))
	in2, _ := s.Record(context.Background(), "team.activity.team_created", "", []byte(`{"x":1}`))
	if !in1 || in2 {
		t.Errorf("fallback id should dedup same payload: in1=%v in2=%v", in1, in2)
	}
}

func TestRecord_RepoError_PropagatedForNak(t *testing.T) {
	r := newFake()
	r.failErr = errors.New("db down")
	s := service.New(r)
	_, err := s.Record(context.Background(), "x", "id-3", []byte(`{}`))
	if err == nil {
		t.Fatal("expected err for NAK path")
	}
}

func TestRecord_OccurredAt_ParsedFromPayload(t *testing.T) {
	r := newFake()
	s := service.New(r)
	payload := []byte(`{"occurred_at":"2026-01-02T03:04:05.123Z","x":1}`)
	_, err := s.Record(context.Background(), "team.activity.team_created", "id-4", payload)
	if err != nil {
		t.Fatal(err)
	}
}
