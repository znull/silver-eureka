package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
)

func newVerboseHTTPClient() *http.Client {
	return &http.Client{
		Transport: &verboseHTTPTransport{},
	}
}

type verboseHTTPTransport struct{}

func (t *verboseHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		reqBody, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
		reqStr, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			return nil, err
		}
		log.Printf("[http transport] sending request to %v\n%s", req.URL, reqStr)
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	} else {
		reqStr, err := httputil.DumpRequestOut(req, false)
		if err != nil {
			return nil, err
		}
		log.Printf("[http transport] sending request to %v\n%s", req.URL, reqStr)
	}

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		log.Printf("[http transport] error: %v", err)
		return nil, err
	}

	respStr, _ := httputil.DumpResponse(resp, false)
	log.Printf("[http transport] received %d response from %v\n%s", resp.StatusCode, req.URL, respStr)
	resp.Body = &verboseHTTPBody{body: resp.Body}

	return resp, nil
}

type verboseHTTPBody struct {
	body io.ReadCloser
}

func (b *verboseHTTPBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		log.Printf("[http transport] received %d bytes of the response:\n%s", n, p[:n])
	}
	return n, err
}

func (b *verboseHTTPBody) Close() error {
	return b.body.Close()
}
