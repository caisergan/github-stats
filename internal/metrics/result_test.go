package metrics

import (
	"encoding/json"
	"testing"
)

func TestTimeSeriesResultJSON(t *testing.T) {
	r := TimeSeries("commits", []Point{{Date: "2026-03-01", Value: 2}, {Date: "2026-03-02", Value: 5}})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "time_series" {
		t.Fatalf("kind = %v, want time_series", got["kind"])
	}
	series, ok := got["series"].([]any)
	if !ok || len(series) != 2 {
		t.Fatalf("series = %v", got["series"])
	}
	first := series[0].(map[string]any)
	if first["date"] != "2026-03-01" || first["value"].(float64) != 2 {
		t.Fatalf("first point = %v", first)
	}
	// scalar/buckets/rows must be omitted.
	if _, present := got["value"]; present {
		t.Fatal("time-series result must not emit scalar value")
	}
}

func TestScalarResultJSON(t *testing.T) {
	r := Scalar("avg_hours", 12.5, "hours", 7)
	b, _ := json.Marshal(r)
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "scalar" {
		t.Fatalf("kind = %v", got["kind"])
	}
	if got["value"].(float64) != 12.5 || got["unit"] != "hours" || got["count"].(float64) != 7 {
		t.Fatalf("scalar = %v", got)
	}
}

func TestBucketsResultJSON(t *testing.T) {
	r := Buckets("open_issue_age", []Bucket{{Label: "<24h", Count: 3}, {Label: "older", Count: 1}})
	b, _ := json.Marshal(r)
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "buckets" {
		t.Fatalf("kind = %v", got["kind"])
	}
	buckets := got["buckets"].([]any)
	if len(buckets) != 2 || buckets[0].(map[string]any)["label"] != "<24h" {
		t.Fatalf("buckets = %v", buckets)
	}
}

func TestLeaderboardResultJSON(t *testing.T) {
	r := Leaderboard("contributors", []LeaderRow{
		{Login: "neo", Commits: 10, Additions: 100, Deletions: 5},
	})
	b, _ := json.Marshal(r)
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "leaderboard" {
		t.Fatalf("kind = %v", got["kind"])
	}
	rows := got["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["login"] != "neo" {
		t.Fatalf("rows = %v", rows)
	}
}
