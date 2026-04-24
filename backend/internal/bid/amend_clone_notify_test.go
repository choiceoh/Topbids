package bid

import (
	"testing"
)

// ResolvePlannedPrice is the 복수예가 algorithm. It picks the 4 most-frequent
// indices across all bid picks and averages the corresponding reserve prices
// into the 예정가. DB-independent so unit tests exercise it directly.

func TestResolvePlannedPrice_AveragesTopFour(t *testing.T) {
	reserves := []float64{
		100, 110, 120, 130, 140,
		150, 160, 170, 180, 190,
		200, 210, 220, 230, 240,
	}
	// 4 bidders each pick 2 indices:
	//   - 0: {0, 1}
	//   - 1: {0, 2}
	//   - 2: {1, 3}
	//   - 3: {0, 4}
	// Vote counts: 0→3, 1→2, 2→1, 3→1, 4→1.
	// Top 4 by (count desc, idx asc): 0(3), 1(2), 2(1), 3(1)
	// → avg of reserves at 0/1/2/3 = (100+110+120+130)/4 = 115
	picks := [][]int{{0, 1}, {0, 2}, {1, 3}, {0, 4}}
	got := ResolvePlannedPrice(reserves, picks)
	if got != 115 {
		t.Errorf("got %v, want 115", got)
	}
}

func TestResolvePlannedPrice_HandlesFewerThanFourUniquePicks(t *testing.T) {
	// Only 2 distinct indices picked — average the 2 instead of 4.
	reserves := []float64{100, 200, 300, 400, 500}
	picks := [][]int{{0, 1}, {0, 1}, {1, 0}}
	got := ResolvePlannedPrice(reserves, picks)
	// Both 0 and 1 had equal counts; ties resolved by lower idx first.
	// Only 2 unique → avg of reserves[0] + reserves[1] = 150.
	if got != 150 {
		t.Errorf("got %v, want 150", got)
	}
}

func TestResolvePlannedPrice_ReturnsZeroWhenEmpty(t *testing.T) {
	if got := ResolvePlannedPrice(nil, [][]int{{0}}); got != 0 {
		t.Errorf("empty reserves: got %v, want 0", got)
	}
	if got := ResolvePlannedPrice([]float64{100}, nil); got != 0 {
		t.Errorf("empty picks: got %v, want 0", got)
	}
	if got := ResolvePlannedPrice([]float64{100}, [][]int{{5}}); got != 0 {
		t.Errorf("all out-of-range picks: got %v, want 0", got)
	}
}

func TestGenerateReservePrices_StaysWithinSpread(t *testing.T) {
	// 15 candidates, each within ±3% of the base. Checks the spread is
	// tight enough that downstream avg isn't wildly off base.
	const base = 10_000_000.0
	out := GenerateReservePrices(base)
	if len(out) != 15 {
		t.Fatalf("want 15 candidates, got %d", len(out))
	}
	min := base * 0.97
	max := base * 1.03
	for i, v := range out {
		if v < min || v > max {
			t.Errorf("candidate[%d] = %v outside [%v, %v]", i, v, min, max)
		}
	}
}
