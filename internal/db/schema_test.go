package db

import (
	"testing"
	"time"
)

func TestSchemaBootstrapTimeout(t *testing.T) {
	t.Setenv("NETPULSE_SCHEMA_TIMEOUT_SEC", "")
	if got := schemaBootstrapTimeout(); got != 180*time.Second {
		t.Fatalf("default timeout = %s, want 180s", got)
	}

	t.Setenv("NETPULSE_SCHEMA_TIMEOUT_SEC", "5")
	if got := schemaBootstrapTimeout(); got != 30*time.Second {
		t.Fatalf("minimum timeout = %s, want 30s", got)
	}

	t.Setenv("NETPULSE_SCHEMA_TIMEOUT_SEC", "60")
	if got := schemaBootstrapTimeout(); got != 60*time.Second {
		t.Fatalf("configured timeout = %s, want 60s", got)
	}

	t.Setenv("NETPULSE_SCHEMA_TIMEOUT_SEC", "3600")
	if got := schemaBootstrapTimeout(); got != 1800*time.Second {
		t.Fatalf("maximum timeout = %s, want 1800s", got)
	}

	t.Setenv("NETPULSE_SCHEMA_TIMEOUT_SEC", "not-a-number")
	if got := schemaBootstrapTimeout(); got != 180*time.Second {
		t.Fatalf("invalid timeout = %s, want 180s", got)
	}
}
