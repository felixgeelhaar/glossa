package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	err := run(context.Background(), nil, lookupFrom(nil), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want a DATABASE_URL error", err)
	}
}

func TestMigrateFlagOverridesEnv(t *testing.T) {
	// -migrate=up without MIGRATION_DATABASE_URL must fail validation,
	// proving the flag reached the config.
	err := run(context.Background(), []string{"-migrate=up"},
		lookupFrom(map[string]string{"DATABASE_URL": "postgres://app@localhost/glossa"}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MIGRATION_DATABASE_URL") {
		t.Fatalf("err = %v, want MIGRATION_DATABASE_URL required", err)
	}
}

func TestVersionFlag(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-version"}, lookupFrom(nil), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Error("no version printed")
	}
}

func TestUnknownFlag(t *testing.T) {
	if err := run(context.Background(), []string{"-nope"}, lookupFrom(nil), &bytes.Buffer{}); err == nil {
		t.Error("unknown flag accepted")
	}
}
