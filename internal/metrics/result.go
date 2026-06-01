package metrics

// ResultKind tags a Result so the frontend can pick one renderer per shape.
type ResultKind string

const (
	KindTimeSeries  ResultKind = "time_series"
	KindScalar      ResultKind = "scalar"
	KindBuckets     ResultKind = "buckets"
	KindLeaderboard ResultKind = "leaderboard"
)

// Point is one (date, value) sample in a time series.
type Point struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Bucket is one named count in a distribution.
type Bucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// LeaderRow is one ranked contributor row.
type LeaderRow struct {
	Login     string `json:"login"`
	Commits   int64  `json:"commits"`
	Additions int64  `json:"additions"`
	Deletions int64  `json:"deletions"`
}

// Result is the JSON-friendly tagged union returned by every metric. Exactly one
// of Series / (Value,Unit,Count) / Buckets / Rows is populated, per Kind.
type Result struct {
	Kind   ResultKind `json:"kind"`
	Label  string     `json:"label,omitempty"`
	Series []Point    `json:"series,omitempty"`

	// Scalar payload.
	Value *float64 `json:"value,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Count *int64   `json:"count,omitempty"`

	Buckets []Bucket    `json:"buckets,omitempty"`
	Rows    []LeaderRow `json:"rows,omitempty"`
}

// TimeSeries builds a time-series Result.
func TimeSeries(label string, series []Point) Result {
	if series == nil {
		series = []Point{}
	}
	return Result{Kind: KindTimeSeries, Label: label, Series: series}
}

// Scalar builds a scalar Result with a unit and a sample count.
func Scalar(label string, value float64, unit string, count int64) Result {
	v, c := value, count
	return Result{Kind: KindScalar, Label: label, Value: &v, Unit: unit, Count: &c}
}

// Buckets builds a distribution Result.
func Buckets(label string, buckets []Bucket) Result {
	if buckets == nil {
		buckets = []Bucket{}
	}
	return Result{Kind: KindBuckets, Label: label, Buckets: buckets}
}

// Leaderboard builds a leaderboard Result.
func Leaderboard(label string, rows []LeaderRow) Result {
	if rows == nil {
		rows = []LeaderRow{}
	}
	return Result{Kind: KindLeaderboard, Label: label, Rows: rows}
}
