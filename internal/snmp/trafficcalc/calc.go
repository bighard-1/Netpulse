package trafficcalc

import (
	"math"
	"strings"
)

func PersistableStatus(status string) bool {
	return status == "VALID" || strings.HasPrefix(status, "DIR_") || status == "CACHE_AVG"
}

func PairHighSpeedCacheSample(pending bool, previous, curr int64, status string) (int64, string, bool) {
	if curr <= 0 || status != "VALID" {
		return curr, status, false
	}
	if !pending || previous <= 0 {
		return curr, "CACHE_WAIT", true
	}
	return (previous + curr) / 2, "CACHE_AVG", false
}

func ChooseCloserToPrev(prev, a, b int64) int64 {
	if prev <= 0 {
		if a <= 0 {
			return b
		}
		if b <= 0 {
			return a
		}
		if a < b {
			return a
		}
		return b
	}
	da := abs64(a - prev)
	db := abs64(b - prev)
	if da <= db {
		return a
	}
	return b
}

func PickCounterPair(mode string, hcIn, hcOut, legacyIn, legacyOut uint64) (uint64, uint64) {
	if mode == "legacy" {
		return legacyIn, legacyOut
	}
	return hcIn, hcOut
}

func SafeDelta(mode string, curr, prev uint64) (uint64, bool) {
	if curr < prev {
		if mode == "legacy" {
			const wrap32 = uint64(1) << 32
			return (wrap32 - prev) + curr, false
		}
		return 0, true
	}
	return curr - prev, false
}

func RawBps(deltaOctets uint64, seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	v := (float64(deltaOctets) * 8) / seconds
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return int64(v)
}

func ClampOrKeepPrev(curr, prev int64, maxReasonableBps float64) int64 {
	if curr <= 0 {
		return 0
	}
	if maxReasonableBps <= 0 {
		return curr
	}
	if float64(curr) > maxReasonableBps {
		if prev > 0 {
			return prev
		}
		return int64(maxReasonableBps)
	}
	return curr
}

func MaxReasonableBpsBySpeed(speedMbps int) float64 {
	if speedMbps > 0 {
		return float64(speedMbps) * 1_000_000 * 1.10
	}
	return 110_000_000_000
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
