package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"strconv"
	"sync"
	"sync/atomic"
)

func newVerboseHTTPClient(next http.RoundTripper) *verboseHTTPTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &verboseHTTPTransport{next: next}
}

type verboseHTTPTransport struct {
	count int32
	next  http.RoundTripper
}

func (t *verboseHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	n := atomic.AddInt32(&t.count, 1)
	tag := fmt.Sprintf("%d", n)

	reqStr, err := httputil.DumpRequestOut(req, false)
	if err != nil {
		return nil, err
	}

	vPrintf(tag, "sending request to %v\n%s", req.URL, reqStr)
	if req.Body != nil {
		req.Body = &verboseHTTPBody{tag: tag + "/request", body: req.Body}
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		vPrintf(tag, "error: %v", err)
		return nil, err
	}

	respStr, _ := httputil.DumpResponse(resp, false)
	vPrintf(tag, "received %d response from %v\n%s", resp.StatusCode, req.URL, respStr)
	resp.Body = &verboseHTTPBody{tag: tag + "/response", body: resp.Body}

	return resp, nil
}

type verboseHTTPBody struct {
	tag  string
	body io.ReadCloser
	pr   verbosePacketReader
}

func (b *verboseHTTPBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		b.pr.Push(b.tag, p[:n])
	}
	return n, err
}

func (b *verboseHTTPBody) Close() error {
	b.pr.Flush(b.tag)
	return b.body.Close()
}

type verbosePacketReader struct {
	l sync.Mutex

	buf       []byte
	sidebands map[byte]*verbosePacketReader

	finished         bool
	countAfterFinish int
}

func (pr *verbosePacketReader) Push(tag string, p []byte) {
	pr.l.Lock()
	defer pr.l.Unlock()

	if pr.finished {
		pr.countAfterFinish += len(p)
		return
	}
	pr.buf = append(pr.buf, p...)
	pr.flush(tag)
}

func (pr *verbosePacketReader) Flush(tag string) {
	pr.l.Lock()
	defer pr.l.Unlock()
	switch {
	case pr.finished && pr.countAfterFinish > 0:
		vPrintf(tag, "%d unprocessed bytes after PACK or error", pr.countAfterFinish)
	case !pr.finished && len(pr.buf) > 0:
		vPrintf(tag, "%d bytes of incomplete pktline", len(pr.buf))
	}
	if pr.sidebands != nil {
		for i, sb := range pr.sidebands {
			sb.Flush(fmt.Sprintf("%s/sideband-%d", tag, i))
		}
	}
}

func (pr *verbosePacketReader) flush(tag string) {
	for {
		if len(pr.buf) < 4 {
			return
		}

		sizeField := pr.buf[:4]

		if bytes.Equal(sizeField, []byte("0000")) {
			vPrintf(tag, "flush packet")
			pr.buf = pr.buf[4:]
			continue
		}

		if bytes.Equal(sizeField, []byte("PACK")) {
			vPrintf(tag, "PACK")
			pr.finished = true
			pr.countAfterFinish += len(pr.buf) - 4
			pr.buf = nil
			return
		}

		size, err := strconv.ParseInt(string(sizeField), 16, 32)
		if err != nil || size < 4 {
			vPrintf(tag, "invalid packet size: %s", sizeField)
			pr.buf = nil
			pr.countAfterFinish += len(pr.buf)
			pr.finished = true
			return
		}

		if size > int64(len(pr.buf)) {
			// we don't have the full pktline yet.
			return
		}

		data := pr.buf[4:size]
		pr.buf = pr.buf[size:]

		sbi := data[0]
		switch sbi {
		case 1, 2:
			sb := pr.getSideband(sbi)
			sb.Push(fmt.Sprintf("%s/sideband-%d", tag, sbi), data[1:])
		default:
			vPrintf(tag, "%04X: %q", size, string(data))
		}
	}
}

func (pr *verbosePacketReader) getSideband(i byte) *verbosePacketReader {
	if pr.sidebands == nil {
		pr.sidebands = make(map[byte]*verbosePacketReader)
	}
	child, ok := pr.sidebands[i]
	if !ok {
		child = &verbosePacketReader{}
		pr.sidebands[i] = child
	}
	return child
}

func vPrintf(tag, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[http transport/%s] %s", tag, msg)
}
