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
