package db

import (
	"testing"
	"time"
)

func TestResolveHistoryBucketIntervalForTrafficRanges(t *testing.T) {
	if got := resolveHistoryBucketInterval(30*24*time.Hour, "1h", 1000, true); got != "1 hours" {
		t.Fatalf("30d requested 1h bucket=%q, want 1 hours", got)
	}
	if got := resolveHistoryBucketInterval(7*24*time.Hour, "15m", 1000, true); got != "15 minutes" {
		t.Fatalf("7d requested 15m bucket=%q, want 15 minutes", got)
	}
	if got := resolveHistoryBucketInterval(60*24*time.Hour, "2h", 1000, true); got != "2 hours" {
		t.Fatalf("60d requested 2h bucket=%q, want 2 hours", got)
	}
	if got := resolveHistoryBucketInterval(365*24*time.Hour, "6h", 1000, true); got != "6 hours" {
		t.Fatalf("365d requested 6h bucket=%q, want 6 hours", got)
	}
	if got := resolveHistoryBucketInterval(60*24*time.Hour, "5m", 1000, true); got != "1 hours" {
		t.Fatalf("60d requested 5m bucket=%q, want 1 hours minimum", got)
	}
}

func TestResolveHistoryBucketIntervalInvalidAndAutoBuckets(t *testing.T) {
	if got := resolveHistoryBucketInterval(12*time.Hour, "bad", 0, false); got != "" {
		t.Fatalf("short span invalid request bucket=%q, want empty raw query", got)
	}
	if got := resolveHistoryBucketInterval(12*time.Hour, "bad", 720, false); got != "1 minutes" {
		t.Fatalf("short span auto bucket=%q, want 1 minutes", got)
	}
	if got := resolveHistoryBucketInterval(2*24*time.Hour, "bad", 2000, false); got != "5 minutes" {
		t.Fatalf("2d auto bucket=%q, want minimum 5 minutes", got)
	}
	if got := resolveHistoryBucketInterval(8*24*time.Hour, "1m", 2000, true); got != "30 minutes" {
		t.Fatalf("8d agg bucket=%q, want minimum 30 minutes", got)
	}
	if got := resolveHistoryBucketInterval(32*24*time.Hour, "15m", 2000, true); got != "1 hours" {
		t.Fatalf("32d agg bucket=%q, want minimum 1 hours", got)
	}
}

func TestSQLIntervalHelpers(t *testing.T) {
	if got := durationToSQLInterval(90 * time.Second); got != "2 minutes" {
		t.Fatalf("90s interval=%q, want 2 minutes", got)
	}
	if got := durationToSQLInterval(90 * time.Minute); got != "2 hours" {
		t.Fatalf("90m interval=%q, want 2 hours", got)
	}
	if got, ok := parseSQLInterval("2 hours"); !ok || got != 2*time.Hour {
		t.Fatalf("parse 2 hours got=%s ok=%v", got, ok)
	}
	if got, ok := parseSQLInterval("15 minutes"); !ok || got != 15*time.Minute {
		t.Fatalf("parse 15 minutes got=%s ok=%v", got, ok)
	}
	if _, ok := parseSQLInterval("bad"); ok {
		t.Fatalf("parse bad interval should fail")
	}
}

func TestDecimateInterfaceHistoryKeepsLastPoint(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	in := make([]InterfaceHistoryPoint, 0, 10)
	for i := 0; i < 10; i++ {
		in = append(in, InterfaceHistoryPoint{Timestamp: start.Add(time.Duration(i) * time.Minute)})
	}
	out := decimateInterfaceHistory(in, time.Hour, 4)
	if len(out) > 6 {
		t.Fatalf("decimated len=%d, want compact output", len(out))
	}
	if !out[len(out)-1].Timestamp.Equal(in[len(in)-1].Timestamp) {
		t.Fatalf("last point was not preserved")
	}
}

func TestDecimateDeviceHistoryKeepsLastPoint(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	in := make([]DeviceHistoryPoint, 0, 10)
	for i := 0; i < 10; i++ {
		in = append(in, DeviceHistoryPoint{Timestamp: start.Add(time.Duration(i) * time.Minute)})
	}
	out := decimateDeviceHistory(in, time.Hour, 4)
	if len(out) > 6 {
		t.Fatalf("decimated len=%d, want compact output", len(out))
	}
	if !out[len(out)-1].Timestamp.Equal(in[len(in)-1].Timestamp) {
		t.Fatalf("last point was not preserved")
	}
}

func TestTrafficRollupChunksPerRun(t *testing.T) {
	t.Setenv("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", "")
	if got := trafficRollupChunksPerRun("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", 4); got != 4 {
		t.Fatalf("default chunks=%d, want 4", got)
	}
	t.Setenv("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", "8")
	if got := trafficRollupChunksPerRun("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", 4); got != 8 {
		t.Fatalf("env chunks=%d, want 8", got)
	}
	t.Setenv("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", "200")
	if got := trafficRollupChunksPerRun("NETPULSE_TRAFFIC_ROLLUP_5M_CHUNKS_PER_RUN", 4); got != 24 {
		t.Fatalf("capped chunks=%d, want 24", got)
	}
}
