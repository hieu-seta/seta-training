package events_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/events"
)

func TestMsgID_Deterministic(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	id1 := events.MsgID(events.SubjMemberAdded, a, b)
	id2 := events.MsgID(events.SubjMemberAdded, a, b)
	if id1 != id2 {
		t.Errorf("not deterministic: %s vs %s", id1, id2)
	}
}

func TestMsgID_DiffersBySubject(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	if events.MsgID(events.SubjMemberAdded, a, b) == events.MsgID(events.SubjMemberRemoved, a, b) {
		t.Error("different subjects collided")
	}
}

func TestMsgID_DiffersByPayload(t *testing.T) {
	a := uuid.New()
	b1 := uuid.New()
	b2 := uuid.New()
	if events.MsgID(events.SubjMemberAdded, a, b1) == events.MsgID(events.SubjMemberAdded, a, b2) {
		t.Error("different uids collided")
	}
}

func TestMsgID_StringPart(t *testing.T) {
	id1 := events.MsgID(events.SubjTeamCreated, uuid.New(), "extra")
	id2 := events.MsgID(events.SubjTeamCreated, uuid.New(), "extra")
	if id1 == id2 {
		t.Error("expected different uuids → diff id")
	}
}

func TestMsgID_LengthFixed(t *testing.T) {
	id := events.MsgID(events.SubjFolderCreated, uuid.New())
	if len(id) != 32 {
		t.Errorf("expected 32-char hex, got %d (%q)", len(id), id)
	}
}
