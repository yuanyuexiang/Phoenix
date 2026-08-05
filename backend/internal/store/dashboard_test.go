package store

import "testing"

func TestNormalizeDashboardDays(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, 7}, {7, 7}, {14, 7}, {30, 30}} {
		if got := normalizeDashboardDays(tc.in); got != tc.want {
			t.Fatalf("normalizeDashboardDays(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
