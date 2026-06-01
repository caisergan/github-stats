package githubapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github-stats/internal/store"
)

// ETagTransport is an http.RoundTripper that performs conditional REST GETs
// using cached ETags. On a 304 it serves the cached body as a 200, so callers
// never see a 304 and the request does not count against the rate limit
// (spec §3). Non-GET requests are passed straight through to Base.
type ETagTransport struct {
	Store *store.Store
	Base  http.RoundTripper
}

func (t *ETagTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper.
func (t *ETagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return t.base().RoundTrip(req)
	}
	ctx := req.Context()
	url := req.URL.String()

	cached, err := t.Store.GetETag(ctx, url)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if cached != nil {
		req = req.Clone(ctx)
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := t.base().RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		resp.Body.Close()
		return t.responseFromCache(req, cached), nil
	}

	if resp.StatusCode == http.StatusOK {
		if etag := resp.Header.Get("ETag"); etag != "" {
			body, rerr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if rerr != nil {
				return nil, rerr
			}
			_ = t.Store.PutETag(ctx, &store.ETagEntry{
				URL:          url,
				ETag:         etag,
				Status:       http.StatusOK,
				Body:         body,
				LastModified: resp.Header.Get("Last-Modified"),
			})
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.ContentLength = int64(len(body))
		}
	}
	return resp, nil
}

func (t *ETagTransport) responseFromCache(req *http.Request, cached *store.ETagEntry) *http.Response {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("ETag", cached.ETag)
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(cached.Body)),
		ContentLength: int64(len(cached.Body)),
		Request:       req,
	}
}
