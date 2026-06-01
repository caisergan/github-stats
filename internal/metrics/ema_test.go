package metrics

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestEMASpan5KnownValues(t *testing.T) {
	in := []float64{10, 12, 14, 16, 18}
	got := EMA(in, 5)
	// alpha = 2/(5+1) = 1/3. Seed = 10.
	// e1 = 10
	// e2 = 12/3 + 10*2/3 = 4 + 6.6666667 = 10.6666667
	// e3 = 14/3 + 10.6666667*2/3 = 4.6666667 + 7.1111111 = 11.7777778
	// e4 = 16/3 + 11.7777778*2/3 = 5.3333333 + 7.8518519 = 13.1851852
	// e5 = 18/3 + 13.1851852*2/3 = 6 + 8.7901235 = 14.7901235
	want := []float64{10, 10.6666666667, 11.7777777778, 13.1851851852, 14.7901234568}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-6 {
			t.Errorf("ema[%d] = %.10f, want %.10f", i, got[i], want[i])
		}
	}
}

func TestEMAEmptyAndSingle(t *testing.T) {
	if got := EMA(nil, 5); len(got) != 0 {
		t.Fatalf("EMA(nil) = %v, want empty", got)
	}
	got := EMA([]float64{7}, 14)
	if len(got) != 1 || !approx(got[0], 7) {
		t.Fatalf("EMA([7]) = %v, want [7]", got)
	}
}

func TestEMAInvalidSpanFallsBackToInput(t *testing.T) {
	in := []float64{1, 2, 3}
	got := EMA(in, 0) // span <= 0 → no smoothing, copy of input
	for i := range in {
		if !approx(got[i], in[i]) {
			t.Fatalf("EMA span 0 should copy input: %v", got)
		}
	}
}

func TestMeanAndMedian(t *testing.T) {
	if !approx(mean([]float64{2, 4, 6}), 4) {
		t.Fatal("mean wrong")
	}
	if !approx(mean(nil), 0) {
		t.Fatal("mean(nil) should be 0")
	}
	if !approx(median([]float64{3, 1, 2}), 2) {
		t.Fatal("median odd wrong")
	}
	if !approx(median([]float64{1, 2, 3, 4}), 2.5) {
		t.Fatal("median even wrong")
	}
	if !approx(median(nil), 0) {
		t.Fatal("median(nil) should be 0")
	}
}
