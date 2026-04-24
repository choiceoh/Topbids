package bid

import (
	"errors"
	"testing"
)

func p(v float64) *float64 { return &v }

func TestPickLowest(t *testing.T) {
	cases := []struct {
		name string
		bids []bidRow
		want string
	}{
		{
			name: "single bid",
			bids: []bidRow{{id: "a", totalAmount: 100}},
			want: "a",
		},
		{
			name: "three bids, pick smallest",
			bids: []bidRow{
				{id: "a", totalAmount: 300},
				{id: "b", totalAmount: 100},
				{id: "c", totalAmount: 200},
			},
			want: "b",
		},
		{
			name: "tie broken by pre-sorted order (first wins)",
			bids: []bidRow{
				{id: "early", totalAmount: 100},
				{id: "late", totalAmount: 100},
			},
			want: "early",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := pickLowest(tc.bids)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPickWeighted_BestTotalScoreWins(t *testing.T) {
	// techWeight=20: total = tech*0.2 + price*0.8
	// Bids:
	//   a: 100 tech=90 → price=100 → total = 18 + 80 = 98
	//   b: 110 tech=95 → price=90.9 → total = 19 + 72.7 = 91.7
	//   c: 200 tech=100 → price=50 → total = 20 + 40 = 60
	// Winner: a
	bids := []bidRow{
		{id: "a", totalAmount: 100, techScore: p(90)},
		{id: "b", totalAmount: 110, techScore: p(95)},
		{id: "c", totalAmount: 200, techScore: p(100)},
	}
	got, _, err := pickWeighted(bids, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a" {
		t.Errorf("got %q, want a", got)
	}
}

func TestPickWeighted_HighTechCanOverturnLowPrice(t *testing.T) {
	// techWeight=60: total = tech*0.6 + price*0.4
	// Bids:
	//   cheap_weak: 100 tech=50 → price=100 → total = 30 + 40 = 70
	//   pricey_strong: 150 tech=95 → price=66.7 → total = 57 + 26.7 = 83.7
	// Winner: pricey_strong (tech weight pushes it above)
	bids := []bidRow{
		{id: "cheap_weak", totalAmount: 100, techScore: p(50)},
		{id: "pricey_strong", totalAmount: 150, techScore: p(95)},
	}
	got, _, err := pickWeighted(bids, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got != "pricey_strong" {
		t.Errorf("got %q, want pricey_strong", got)
	}
}

func TestPickWeighted_MissingTechScoreFails(t *testing.T) {
	bids := []bidRow{
		{id: "a", totalAmount: 100, techScore: p(90)},
		{id: "b", totalAmount: 110, techScore: nil}, // missing!
	}
	_, _, err := pickWeighted(bids, 20)
	if !errors.Is(err, ErrTechScoreMissing) {
		t.Errorf("got %v, want ErrTechScoreMissing", err)
	}
}

func TestPickWeighted_SingleBid(t *testing.T) {
	bids := []bidRow{{id: "only", totalAmount: 100, techScore: p(80)}}
	got, _, err := pickWeighted(bids, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got != "only" {
		t.Errorf("got %q, want only", got)
	}
}

func TestPickWinner_Dispatch(t *testing.T) {
	bids := []bidRow{
		{id: "a", totalAmount: 100, techScore: p(50)},
		{id: "b", totalAmount: 50, techScore: p(90)},
	}

	// lowest: b (50) wins on price
	got, _, err := pickWinner(bids, EvalMethodLowest)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Errorf("lowest: got %q, want b", got)
	}

	// weighted (tw=20): both have low prices; b has min=50 → price=100, a price=50
	// a: 50*0.2 + 50*0.8 = 10 + 40 = 50
	// b: 90*0.2 + 100*0.8 = 18 + 80 = 98
	// Winner: b
	got, _, err = pickWinner(bids, EvalMethodWeighted)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Errorf("weighted: got %q, want b", got)
	}

	// Unknown method falls back to lowest (b).
	got, _, err = pickWinner(bids, "nonsense")
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Errorf("fallback: got %q, want b", got)
	}
}

func TestScoreOf_ZeroAmountIsSafe(t *testing.T) {
	b := bidRow{id: "x", totalAmount: 0, techScore: p(80)}
	s := scoreOf(b, 0, 0.2, 0.8)
	// price component = 0 (guarded by totalAmount > 0). tech contribution = 80*0.2 = 16.
	if s != 16 {
		t.Errorf("got %v, want 16", s)
	}
}

func TestApplyMinWinFloor_DropsLowballBids(t *testing.T) {
	// 예정가 1억 · 하한율 80% → 8천만 원 미만 bid는 실격.
	estimated := 100_000_000.0
	rate := 0.80
	bids := []bidRow{
		{id: "low", totalAmount: 70_000_000},
		{id: "ok1", totalAmount: 85_000_000},
		{id: "ok2", totalAmount: 90_000_000},
		{id: "zero", totalAmount: 0},
	}
	out := applyMinWinFloor(bids, &estimated, &rate)
	if len(out) != 2 {
		t.Fatalf("want 2 survivors, got %d", len(out))
	}
	for _, b := range out {
		if b.id == "low" || b.id == "zero" {
			t.Errorf("bid %s should have been filtered out", b.id)
		}
	}
}

func TestApplyMinWinFloor_InactiveWithoutConfig(t *testing.T) {
	bids := []bidRow{{id: "a", totalAmount: 100}}
	// nil config → opt-in feature stays off.
	if out := applyMinWinFloor(bids, nil, nil); len(out) != 1 {
		t.Error("nil configs should leave bids unchanged")
	}
	zero := 0.0
	rate := 0.80
	if out := applyMinWinFloor(bids, &zero, &rate); len(out) != 1 {
		t.Error("zero estimated_price should leave bids unchanged")
	}
	est := 1_000.0
	zeroRate := 0.0
	if out := applyMinWinFloor(bids, &est, &zeroRate); len(out) != 1 {
		t.Error("zero rate should leave bids unchanged")
	}
}
