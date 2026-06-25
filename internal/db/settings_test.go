package db

import "testing"

func TestClampIntSetting(t *testing.T) {
	if got := clampIntSetting(2, 5, 1440); got != 5 {
		t.Fatalf("clamp below min=%d, want 5", got)
	}
	if got := clampIntSetting(180, 5, 1440); got != 180 {
		t.Fatalf("clamp valid=%d, want 180", got)
	}
	if got := clampIntSetting(2000, 5, 1440); got != 1440 {
		t.Fatalf("clamp above max=%d, want 1440", got)
	}
}
