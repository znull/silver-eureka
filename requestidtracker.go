package main

import (
	"log"
	"net/http"
)

func newRequestIDTracker(next http.RoundTripper) *requestIDTracker {
	if next == nil {
		next = http.DefaultTransport
	}
	return &requestIDTracker{next: next}
}

type requestIDTracker struct {
	next http.RoundTripper
}

func (t *requestIDTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if resp != nil {
		log.Printf("[http] {%s} %d %s %s",
			resp.Header.Get("X-GitHub-Request-ID"),
			resp.StatusCode, req.Method, req.URL)
	}
	return resp, err
}
