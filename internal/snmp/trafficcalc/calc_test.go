package trafficcalc

import "testing"

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
			gotDelta, gotReset := SafeDelta(tt.mode, tt.curr, tt.prev)
			if gotDelta != tt.wantDelta || gotReset != tt.wantReset {
				t.Fatalf("SafeDelta()=(%d,%v), want (%d,%v)", gotDelta, gotReset, tt.wantDelta, tt.wantReset)
			}
		})
	}
}

func TestRawBps(t *testing.T) {
	if got := RawBps(125_000, 1); got != 1_000_000 {
		t.Fatalf("RawBps()=%d, want 1000000", got)
	}
	if got := RawBps(125_000, 0); got != 0 {
		t.Fatalf("RawBps zero seconds=%d, want 0", got)
	}
	if got := RawBps(125_000, -1); got != 0 {
		t.Fatalf("RawBps negative seconds=%d, want 0", got)
	}
}

func TestPickCounterPair(t *testing.T) {
	in, out := PickCounterPair("legacy", 1, 2, 3, 4)
	if in != 3 || out != 4 {
		t.Fatalf("legacy pair=(%d,%d), want (3,4)", in, out)
	}
	in, out = PickCounterPair("hc", 1, 2, 3, 4)
	if in != 1 || out != 2 {
		t.Fatalf("hc pair=(%d,%d), want (1,2)", in, out)
	}
}

func TestMaxReasonableBpsBySpeed(t *testing.T) {
	if got := MaxReasonableBpsBySpeed(1000); got != 1_100_000_000 {
		t.Fatalf("MaxReasonableBpsBySpeed(1000)=%f, want 1100000000", got)
	}
	if got := MaxReasonableBpsBySpeed(0); got != 110_000_000_000 {
		t.Fatalf("MaxReasonableBpsBySpeed(0)=%f, want 110000000000", got)
	}
}

func TestPairHighSpeedCacheSampleWaitsThenAverages(t *testing.T) {
	got, status, pending := PairHighSpeedCacheSample(false, 0, 900, "VALID")
	if got != 900 || status != "CACHE_WAIT" || !pending {
		t.Fatalf("first sample=(%d,%s,%v), want (900,CACHE_WAIT,true)", got, status, pending)
	}
	got, status, pending = PairHighSpeedCacheSample(true, 900, 300, "VALID")
	if got != 600 || status != "CACHE_AVG" || pending {
		t.Fatalf("second sample=(%d,%s,%v), want (600,CACHE_AVG,false)", got, status, pending)
	}
}
