package metrics

import "sort"

// EMA returns the exponential moving average of values with the given span
// (e.g. 5 or 14). alpha = 2/(span+1); the series is seeded with the first value.
// A span <= 0 disables smoothing and returns a copy of the input.
func EMA(values []float64, span int) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	if span <= 0 {
		copy(out, values)
		return out
	}
	alpha := 2.0 / (float64(span) + 1.0)
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out
}

// mean returns the arithmetic mean, or 0 for an empty slice.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// median returns the median, or 0 for an empty slice. It sorts a copy.
func median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, values)
	sort.Float64s(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}
