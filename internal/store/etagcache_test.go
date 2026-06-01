package store

import (
	"bytes"
	"context"
	"testing"
)

func TestETagCachePutGet(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	if _, err := s.GetETag(ctx, "https://api/x"); err != ErrNotFound {
		t.Fatalf("absent etag got %v, want ErrNotFound", err)
	}

	if err := s.PutETag(ctx, &ETagEntry{
		URL: "https://api/x", ETag: `W/"abc"`, Status: 200,
		Body: []byte(`{"ok":true}`), LastModified: "Mon, 01 Jan 2026 00:00:00 GMT",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetETag(ctx, "https://api/x")
	if err != nil {
		t.Fatal(err)
	}
	if got.ETag != `W/"abc"` || got.Status != 200 || !bytes.Equal(got.Body, []byte(`{"ok":true}`)) {
		t.Fatalf("etag round trip mismatch: %+v", got)
	}

	// Put again with new content overwrites by URL.
	if err := s.PutETag(ctx, &ETagEntry{
		URL: "https://api/x", ETag: `W/"def"`, Status: 200, Body: []byte(`{"ok":false}`),
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetETag(ctx, "https://api/x")
	if got.ETag != `W/"def"` || !bytes.Equal(got.Body, []byte(`{"ok":false}`)) {
		t.Fatalf("etag not overwritten: %+v", got)
	}

	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM etags`).Scan(&n)
	if n != 1 {
		t.Fatalf("etag rows = %d, want 1", n)
	}
}
