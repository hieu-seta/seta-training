package logger_test

import (
	"testing"

	"github.com/hieu-seta/seta-training/pkg/logger"
)

func TestIsBanned(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"Password", true},
		{"user_password", true},
		{"pwd", true},
		{"access_token", true},
		{"refresh_token", true},
		{"Authorization", true},
		{"api_key", true},
		{"apikey", true},
		{"username", false},
		{"email", false},
		{"id", false},
		{"name", false},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			if got := logger.IsBanned(tc.key); got != tc.want {
				t.Errorf("IsBanned(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestRedact_TopLevel(t *testing.T) {
	in := map[string]any{
		"email":    "a@b.c",
		"password": "hunter2",
		"token":    "jwt.xxx",
		"id":       42,
	}
	out := logger.Redact(in)
	if out["email"] != "a@b.c" {
		t.Errorf("email mutated: %v", out["email"])
	}
	if out["id"] != 42 {
		t.Errorf("id mutated: %v", out["id"])
	}
	if out["password"] != logger.Mask {
		t.Errorf("password not masked: %v", out["password"])
	}
	if out["token"] != logger.Mask {
		t.Errorf("token not masked: %v", out["token"])
	}
}

func TestRedact_Nested(t *testing.T) {
	in := map[string]any{
		"user": map[string]any{
			"name":          "alice",
			"refresh_token": "rt.xxx",
		},
	}
	out := logger.Redact(in)
	u, ok := out["user"].(map[string]any)
	if !ok {
		t.Fatalf("user not a map: %T", out["user"])
	}
	if u["name"] != "alice" {
		t.Errorf("name mutated: %v", u["name"])
	}
	if u["refresh_token"] != logger.Mask {
		t.Errorf("refresh_token not masked: %v", u["refresh_token"])
	}
}

func TestRedact_SliceOfMaps(t *testing.T) {
	in := map[string]any{
		"items": []any{
			map[string]any{"id": 1, "secret": "s1"},
			map[string]any{"id": 2, "secret": "s2"},
		},
	}
	out := logger.Redact(in)
	items, ok := out["items"].([]any)
	if !ok {
		t.Fatalf("items not a slice: %T", out["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			t.Fatalf("items[%d] not a map: %T", i, it)
		}
		if m["secret"] != logger.Mask {
			t.Errorf("items[%d].secret not masked: %v", i, m["secret"])
		}
	}
}

func TestRedact_Nil(t *testing.T) {
	if got := logger.Redact(nil); got != nil {
		t.Errorf("Redact(nil) = %v, want nil", got)
	}
}

func TestRedact_DoesNotMutateInput(t *testing.T) {
	in := map[string]any{"password": "hunter2"}
	_ = logger.Redact(in)
	if in["password"] != "hunter2" {
		t.Errorf("input mutated: %v", in["password"])
	}
}
