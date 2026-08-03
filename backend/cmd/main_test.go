package main

import (
	"strings"
	"testing"
)

func TestLoadEnvironmentSkipsDotEnvOnRender(t *testing.T) {
	t.Setenv("RENDER", "true")

	if err := loadEnvironment(); err != nil {
		t.Fatalf("loadEnvironment() on Render returned an error: %v", err)
	}
}

func TestResolveDBURLFromParts(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "bank ledger")
	t.Setenv("DB_USER", "ledger user")
	t.Setenv("DB_PASSWORD", "p@ss/word")
	t.Setenv("DB_SSLMODE", "require")

	got := resolveDBURLFromParts()
	want := "postgresql://ledger%20user:p%40ss%2Fword@localhost:5433/bank%20ledger?sslmode=require"
	if got != want {
		t.Fatalf("resolveDBURLFromParts() = %q, want %q", got, want)
	}
}

func TestResolveDBURLPrefersParts(t *testing.T) {
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "simple_ledger")
	t.Setenv("DB_USER", "root")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("DB_URL", "postgresql://ignored:ignored@example.com:5432/ignored")

	got := resolveDBURL()
	if !strings.Contains(got, "root:secret@db:5432/simple_ledger") {
		t.Fatalf("resolveDBURL() did not prefer split database settings: %q", got)
	}
}

func TestResolveDBURLIgnoresLocalSettingsOnRender(t *testing.T) {
	t.Setenv("RENDER", "true")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "simple_ledger")
	t.Setenv("DB_USER", "root")
	t.Setenv("DB_PASSWORD", "local-only")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("DB_URL", "postgresql://root:local-only@localhost:5432/simple_ledger")
	t.Setenv("INTERNAL_DATABASE_URL", "postgresql://managed:secret@render-db.internal:5432/ledger")
	t.Setenv("RENDER_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	got := resolveDBURL()
	want := "postgresql://managed:secret@render-db.internal:5432/ledger"
	if got != want {
		t.Fatalf("resolveDBURL() = %q, want managed Render URL", got)
	}
}

func TestDemoSeedConfiguration(t *testing.T) {
	t.Run("disabled seed does not require a password", func(t *testing.T) {
		t.Setenv("DEMO_SEED", "false")
		t.Setenv("DEMO_SEED_PASSWORD", "")
		if demoSeedRequested() {
			t.Fatal("demo seed should be disabled")
		}
	})

	t.Run("enabled seed rejects a short password without starting", func(t *testing.T) {
		t.Setenv("DEMO_SEED", "true")
		if !demoSeedRequested() {
			t.Fatal("demo seed should be requested")
		}
		if validSeedPassword("too-short") {
			t.Fatal("short demo seed password should be rejected")
		}
	})

	t.Run("enabled seed accepts a valid password", func(t *testing.T) {
		t.Setenv("DEMO_SEED", " TRUE ")
		if !demoSeedRequested() || !validSeedPassword("integration-only-secret") {
			t.Fatal("valid demo seed configuration should be accepted")
		}
	})
}
