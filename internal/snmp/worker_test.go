package snmp

import (
	"testing"
	"time"
)

func TestInMuteWindow(t *testing.T) {
	base := time.Date(2026, 5, 8, 23, 30, 0, 0, time.UTC)
	if !inMuteWindow("23:00", "07:00", base) {
		t.Fatalf("expected in mute window for cross-day range")
	}
	out := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	if inMuteWindow("23:00", "07:00", out) {
		t.Fatalf("expected outside mute window")
	}
	if !inMuteWindow("00:00", "00:00", out) {
		t.Fatalf("same start/end should be full-day mute")
	}
	if inMuteWindow("bad", "07:00", out) {
		t.Fatalf("invalid input should not mute")
	}
}

func TestPickCounterMode(t *testing.T) {
	w := &Worker{modes: map[string]string{}}
	if got := w.pickCounterMode("d1:p1", 10_000, "2c", true, true); got != "hc" {
		t.Fatalf("high-speed v2c mode=%q, want hc", got)
	}
	if got := w.pickCounterMode("d1:p2", 10_000, "1", true, true); got != "legacy" {
		t.Fatalf("snmp v1 mode=%q, want legacy", got)
	}
	if got := w.pickCounterMode("d1:p3", 10, "", false, true); got != "legacy" {
		t.Fatalf("legacy-only mode=%q, want legacy", got)
	}
}

func newTestWorker() *Worker {
	return &Worker{
		last:  map[string]counterState{},
		modes: map[string]string{},
	}
}

func mustInt64Ptr(t *testing.T, got *int64, want int64) {
	t.Helper()
	if got == nil {
		t.Fatalf("got nil, want %d", want)
	}
	if *got != want {
		t.Fatalf("got %d, want %d", *got, want)
	}
}

func TestCalcBpsInitializesBeforePersisting(t *testing.T) {
	w := newTestWorker()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	in, out, inStatus, outStatus := w.calcBps(1, 10, 1000, 2000, true, 0, 0, false, now, 100, 1000, "2c", 30*time.Second, true)
	if in != nil || out != nil {
		t.Fatalf("initial sample should not persist traffic, got in=%v out=%v", in, out)
	}
	if inStatus != "INITIALIZING" || outStatus != "INITIALIZING" {
		t.Fatalf("status=(%s,%s), want INITIALIZING", inStatus, outStatus)
	}
}

func TestCalcBpsUsesElapsedTimeBetweenCounterChanges(t *testing.T) {
	w := newTestWorker()
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	w.calcBps(1, 10, 1_000, 2_000, true, 0, 0, false, base, 100, 100, "2c", 30*time.Second, true)

	in, out, inStatus, outStatus := w.calcBps(1, 10, 126_000, 252_000, true, 0, 0, false, base.Add(30*time.Second), 130, 100, "2c", 30*time.Second, true)
	if inStatus != "VALID" || outStatus != "VALID" {
		t.Fatalf("status=(%s,%s), want VALID", inStatus, outStatus)
	}
	mustInt64Ptr(t, in, 33_333)
	mustInt64Ptr(t, out, 66_666)
}

func TestCalcBpsSkipsCounterStalePollThenAveragesOverChangeWindow(t *testing.T) {
	w := newTestWorker()
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	w.calcBps(1, 10, 1_000, 2_000, true, 0, 0, false, base, 100, 100, "2c", 30*time.Second, true)

	in, out, inStatus, outStatus := w.calcBps(1, 10, 1_000, 2_000, true, 0, 0, false, base.Add(30*time.Second), 130, 100, "2c", 30*time.Second, true)
	if inStatus != "VALID" || outStatus != "VALID" || in == nil || out == nil {
		t.Fatalf("stale poll should keep a valid zero rate, got in=%v out=%v status=(%s,%s)", in, out, inStatus, outStatus)
	}
	mustInt64Ptr(t, in, 0)
	mustInt64Ptr(t, out, 0)

	in, out, inStatus, outStatus = w.calcBps(1, 10, 241_000, 362_000, true, 0, 0, false, base.Add(60*time.Second), 160, 100, "2c", 30*time.Second, true)
	if inStatus != "VALID" || outStatus != "VALID" {
		t.Fatalf("status=(%s,%s), want VALID after counter changes", inStatus, outStatus)
	}
	// Delta is calculated from the last real counter change at base, not from
	// the stale poll at base+30s: (241000-1000)*8/60 = 32000.
	mustInt64Ptr(t, in, 32_000)
	mustInt64Ptr(t, out, 48_000)
}

func TestCalcBpsReturnsEmptySampleWhenPortDown(t *testing.T) {
	w := newTestWorker()
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	w.calcBps(1, 10, 1_000, 2_000, true, 0, 0, false, base, 100, 100, "2c", 30*time.Second, true)
	w.calcBps(1, 10, 126_000, 252_000, true, 0, 0, false, base.Add(30*time.Second), 130, 100, "2c", 30*time.Second, true)

	in, out, inStatus, outStatus := w.calcBps(1, 10, 126_000, 252_000, true, 0, 0, false, base.Add(60*time.Second), 160, 100, "2c", 30*time.Second, false)
	if in != nil || out != nil {
		t.Fatalf("down port should not persist traffic, got in=%v out=%v", in, out)
	}
	if inStatus != "PORT_DOWN" || outStatus != "PORT_DOWN" {
		t.Fatalf("status=(%s,%s), want PORT_DOWN", inStatus, outStatus)
	}
}

func TestCalcBpsDetectsDeviceRebootAndResetsBaseline(t *testing.T) {
	w := newTestWorker()
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	w.calcBps(1, 10, 10_000, 20_000, true, 0, 0, false, base, 500, 1000, "2c", 30*time.Second, true)

	in, out, inStatus, outStatus := w.calcBps(1, 10, 1_000, 2_000, true, 0, 0, false, base.Add(30*time.Second), 10, 1000, "2c", 30*time.Second, true)
	if in != nil || out != nil {
		t.Fatalf("reboot sample should not persist traffic, got in=%v out=%v", in, out)
	}
	if inStatus != "DEVICE_REBOOT" || outStatus != "DEVICE_REBOOT" {
		t.Fatalf("status=(%s,%s), want DEVICE_REBOOT", inStatus, outStatus)
	}
}

func TestCalcBpsRejectsUnexpectedPollWindowGap(t *testing.T) {
	w := newTestWorker()
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	w.calcBps(1, 10, 10_000, 20_000, true, 0, 0, false, base, 100, 1000, "2c", 30*time.Second, true)

	in, out, inStatus, outStatus := w.calcBps(1, 10, 20_000, 40_000, true, 0, 0, false, base.Add(4*time.Minute), 340, 1000, "2c", 30*time.Second, true)
	if in != nil || out != nil {
		t.Fatalf("window gap should not persist traffic, got in=%v out=%v", in, out)
	}
	if inStatus != "WINDOW_GAP" || outStatus != "WINDOW_GAP" {
		t.Fatalf("status=(%s,%s), want WINDOW_GAP", inStatus, outStatus)
	}
}
