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
