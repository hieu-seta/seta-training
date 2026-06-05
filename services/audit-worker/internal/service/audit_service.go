// Package service orchestrates audit event recording.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/model"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/repo"
	"gorm.io/datatypes"
)

// AuditService records incoming events. Idempotent via idem_key.
type AuditService struct {
	repo  repo.AuditRepo
	newID func() uuid.UUID
	now   func() time.Time
}

// New builds an AuditService.
func New(r repo.AuditRepo) *AuditService {
	return &AuditService{repo: r, newID: uuid.New, now: time.Now}
}

// Record persists one event. Returns (inserted bool, err).
//   - inserted=true: new row written
//   - inserted=false, err=nil: dedup hit, safe to ack
//   - err != nil: caller should NAK (NATS will redeliver)
func (s *AuditService) Record(ctx context.Context, subject, msgID string, payload []byte) (bool, error) {
	if msgID == "" {
		// Fallback: deterministic id from subj+payload.
		msgID = fallbackID(subject, payload)
	}
	// Parse generic fields if present.
	var parsed map[string]any
	_ = json.Unmarshal(payload, &parsed) // tolerate bad json — payload kept raw

	occurredAt := s.now()
	if parsed != nil {
		if t, ok := parsed["occurred_at"].(string); ok {
			if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
				occurredAt = ts
			}
		}
	}
	var actor *uuid.UUID
	if parsed != nil {
		if v, ok := parsed["actor"].(string); ok {
			if id, err := uuid.Parse(v); err == nil {
				actor = &id
			}
		} else if v, ok := parsed["created_by"].(string); ok {
			if id, err := uuid.Parse(v); err == nil {
				actor = &id
			}
		}
	}

	e := &model.Event{
		ID:         s.newID(),
		Subject:    subject,
		IdemKey:    msgID,
		OccurredAt: occurredAt,
		ActorUID:   actor,
		Payload:    datatypes.JSON(payload),
	}
	inserted, err := s.repo.Insert(ctx, e)
	if err != nil {
		return false, err
	}
	return inserted, nil
}

func fallbackID(subject string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(subject))
	h.Write([]byte{0})
	h.Write(payload)
	return "fallback:" + hex.EncodeToString(h.Sum(nil)[:16])
}

// ErrTransient is wrapped by callers to signal NAK-worthy failures.
var ErrTransient = errors.New("transient")
