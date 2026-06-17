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

func TestSafeDelta(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		curr      uint64
		prev      uint64
		wantDelta uint64
		wantReset bool
	}{
		{name: "normal hc increase", mode: "hc", curr: 200, prev: 100, wantDelta: 100},
		{name: "normal legacy increase", mode: "legacy", curr: 200, prev: 100, wantDelta: 100},
		{name: "legacy wrap", mode: "legacy", curr: 10, prev: (uint64(1) << 32) - 5, wantDelta: 15},
		{name: "hc decrease is reset", mode: "hc", curr: 10, prev: 20, wantDelta: 0, wantReset: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDelta, gotReset := safeDelta(tt.mode, tt.curr, tt.prev)
			if gotDelta != tt.wantDelta || gotReset != tt.wantReset {
				t.Fatalf("safeDelta()=(%d,%v), want (%d,%v)", gotDelta, gotReset, tt.wantDelta, tt.wantReset)
			}
		})
	}
}

func TestRawBps(t *testing.T) {
	if got := rawBps(125_000, 1); got != 1_000_000 {
		t.Fatalf("rawBps()=%d, want 1000000", got)
	}
	if got := rawBps(125_000, 0); got != 0 {
		t.Fatalf("rawBps zero seconds=%d, want 0", got)
	}
	if got := rawBps(125_000, -1); got != 0 {
		t.Fatalf("rawBps negative seconds=%d, want 0", got)
	}
}

func TestPickCounterPair(t *testing.T) {
	in, out := pickCounterPair("legacy", 1, 2, 3, 4)
	if in != 3 || out != 4 {
		t.Fatalf("legacy pair=(%d,%d), want (3,4)", in, out)
	}
	in, out = pickCounterPair("hc", 1, 2, 3, 4)
	if in != 1 || out != 2 {
		t.Fatalf("hc pair=(%d,%d), want (1,2)", in, out)
	}
}

func TestMaxReasonableBpsBySpeed(t *testing.T) {
	if got := maxReasonableBpsBySpeed(1000); got != 1_100_000_000 {
		t.Fatalf("maxReasonableBpsBySpeed(1000)=%f, want 1100000000", got)
	}
	if got := maxReasonableBpsBySpeed(0); got != 110_000_000_000 {
		t.Fatalf("maxReasonableBpsBySpeed(0)=%f, want 110000000000", got)
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
