package metrics

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// ErrUnknownMetric is returned (wrapped) by Compute when a requested key is not
// registered, so callers can map it to a 400 with errors.Is rather than matching
// on the error string.
var ErrUnknownMetric = errors.New("unknown metric key")

// Metric is a single, self-contained statistic generator (spec §7). Compute reads
// ONLY from the Source port — never from GitHub or HTTP.
type Metric interface {
	Key() string
	Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error)
}

// Registry maps metric key → Metric. Adding a stat is one Register call.
type Registry struct {
	metrics map[string]Metric
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{metrics: make(map[string]Metric)}
}

// Register adds a metric, overwriting any prior metric with the same key.
func (r *Registry) Register(m Metric) {
	r.metrics[m.Key()] = m
}

// Get returns the metric for a key.
func (r *Registry) Get(key string) (Metric, bool) {
	m, ok := r.metrics[key]
	return m, ok
}

// Keys returns all registered keys, sorted.
func (r *Registry) Keys() []string {
	keys := make([]string, 0, len(r.metrics))
	for k := range r.metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Compute runs the requested metrics (all registered, sorted, when keys is empty)
// and returns key → Result. An unknown key is an error.
func (r *Registry) Compute(ctx context.Context, src Source, repoID int64, keys []string, w Window, opts Opts) (map[string]Result, error) {
	if len(keys) == 0 {
		keys = r.Keys()
	}
	out := make(map[string]Result, len(keys))
	for _, key := range keys {
		m, ok := r.metrics[key]
		if !ok {
			return nil, fmt.Errorf("unknown metric %q: %w", key, ErrUnknownMetric)
		}
		res, err := m.Compute(ctx, src, repoID, w, opts)
		if err != nil {
			return nil, fmt.Errorf("metric %q: %w", key, err)
		}
		out[key] = res
	}
	return out, nil
}
