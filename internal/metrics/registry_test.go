package metrics

import (
	"context"
	"testing"
)

// stubMetric is a trivial Metric for registry tests.
type stubMetric struct {
	key string
	res Result
}

func (m stubMetric) Key() string { return m.key }
func (m stubMetric) Compute(_ context.Context, _ Source, _ int64, _ Window, _ Opts) (Result, error) {
	return m.res, nil
}

func TestRegistryRegisterGetKeys(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	reg.Register(stubMetric{key: "b", res: Scalar("b", 2, "", 0)})

	if _, ok := reg.Get("a"); !ok {
		t.Fatal("Get(a) missing")
	}
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
	keys := reg.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() = %v, want 2 sorted", keys)
	}
	if keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("Keys() not sorted: %v", keys)
	}
}

func TestRegistryComputeSelectedKeys(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	reg.Register(stubMetric{key: "b", res: Scalar("b", 2, "", 0)})

	out, err := reg.Compute(context.Background(), &fakeSource{}, 1, []string{"a"}, Window{From: "2026-03-01", To: "2026-03-31"}, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("computed %d results, want 1", len(out))
	}
	if out["a"].Value == nil || *out["a"].Value != 1 {
		t.Fatalf("a result = %+v", out["a"])
	}
}

func TestRegistryComputeAllWhenNoKeys(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	reg.Register(stubMetric{key: "b", res: Scalar("b", 2, "", 0)})

	out, err := reg.Compute(context.Background(), &fakeSource{}, 1, nil, Window{From: "2026-03-01", To: "2026-03-31"}, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("computed %d results, want all 2", len(out))
	}
}

func TestRegistryComputeUnknownKeyErrors(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	if _, err := reg.Compute(context.Background(), &fakeSource{}, 1, []string{"nope"}, Window{}, Opts{}); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
