package api

import (
	"testing"
	"time"
)

func TestHistoryRangeLabel(t *testing.T) {
	cases := []struct {
		name string
		span time.Duration
		want string
	}{
		{name: "short", span: 6 * time.Hour, want: "today_or_custom_short"},
		{name: "today", span: 24 * time.Hour, want: "today_or_custom_short"},
		{name: "seven days", span: 7 * 24 * time.Hour, want: "7d"},
		{name: "thirty days", span: 30 * 24 * time.Hour, want: "30d"},
		{name: "one year", span: 365 * 24 * time.Hour, want: "1y"},
		{name: "long", span: 500 * 24 * time.Hour, want: "long_range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := historyRangeLabel(tc.span); got != tc.want {
				t.Fatalf("historyRangeLabel(%s)=%q, want %q", tc.span, got, tc.want)
			}
		})
	}
}

func TestSampledIntervalForSource(t *testing.T) {
	if got := sampledIntervalForSource("", "traffic_5m"); got != "5m(预聚合)" {
		t.Fatalf("5m source interval=%q", got)
	}
	if got := sampledIntervalForSource("", "traffic_1h"); got != "1h(预聚合)" {
		t.Fatalf("1h source interval=%q", got)
	}
	if got := sampledIntervalForSource("", "traffic_trends_1h"); got != "1h(趋势归档)" {
		t.Fatalf("trend source interval=%q", got)
	}
	if got := sampledIntervalForSource("15m", "traffic_5m"); got != "15m" {
		t.Fatalf("explicit interval=%q", got)
	}
}
