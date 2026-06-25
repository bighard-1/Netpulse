package api

import "testing"

func TestConverters(t *testing.T) {
	if v, err := toInt64("42"); err != nil || v != 42 {
		t.Fatalf("toInt64 failed: v=%v err=%v", v, err)
	}
	if v, err := toFloat64("3.5"); err != nil || v != 3.5 {
		t.Fatalf("toFloat64 failed: v=%v err=%v", v, err)
	}
	if !toBool("true") {
		t.Fatalf("toBool true string failed")
	}
	if toBool("false") {
		t.Fatalf("toBool false string failed")
	}
}

func TestRuntimeIntUsesFallbackForUnsetValues(t *testing.T) {
	if got := runtimeInt(0, 180); got != 180 {
		t.Fatalf("runtimeInt unset=%d, want fallback 180", got)
	}
	if got := runtimeInt(30, 180); got != 30 {
		t.Fatalf("runtimeInt explicit=%d, want 30", got)
	}
}
