package logger

import "strings"

// Mask is the placeholder substituted in place of sensitive values.
const Mask = "***"

// bannedFields are case-insensitive field names whose values must never reach logs.
// Match is on full lowercased key OR any substring of it (e.g. "user_password" still hits).
var bannedFields = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"access",
	"refresh",
	"authorization",
	"api_key",
	"apikey",
	"private_key",
}

// IsBanned reports whether key contains any banned substring (case-insensitive).
func IsBanned(key string) bool {
	k := strings.ToLower(key)
	for _, b := range bannedFields {
		if strings.Contains(k, b) {
			return true
		}
	}
	return false
}

// Redact returns a shallow copy of m with banned keys masked.
// Nested maps are walked recursively. Other types are passed through unchanged.
func Redact(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsBanned(k) {
			out[k] = Mask
			continue
		}
		switch vv := v.(type) {
		case map[string]any:
			out[k] = Redact(vv)
		case []any:
			out[k] = redactSlice(vv)
		default:
			out[k] = v
		}
	}
	return out
}

func redactSlice(s []any) []any {
	out := make([]any, len(s))
	for i, v := range s {
		switch vv := v.(type) {
		case map[string]any:
			out[i] = Redact(vv)
		case []any:
			out[i] = redactSlice(vv)
		default:
			out[i] = v
		}
	}
	return out
}
