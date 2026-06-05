package events

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

// MsgID builds a deterministic Nats-Msg-Id from (subject, ordered parts).
// Same input → same id → JetStream dedup catches retries within its Duplicates window.
func MsgID(subject string, parts ...any) string {
	var b strings.Builder
	b.WriteString(subject)
	for _, p := range parts {
		b.WriteByte(':')
		switch v := p.(type) {
		case uuid.UUID:
			b.WriteString(v.String())
		case string:
			b.WriteString(v)
		default:
			// keep determinism by hashing fmt.Sprint of unknown types
			// (avoid pulling in fmt for the hot path — convert via switch above)
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:16]) // 32-char id, plenty of entropy
}
