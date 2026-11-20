// Package httpcache provides an on-disk, always-revalidating cache for HTTP GETs.
package httpcache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	maxBodyBytes = 8 << 20
	maxFileBytes = 12 << 20 // JSON base64 expansion plus room for headers.
)

// New wraps a non-nil base with a private disk cache. Use a dedicated directory
// and a base that does not vary credentials or representation headers behind
// the cache (e.g. put an auth-injecting transport outside it).
func New(base http.RoundTripper, dir string) (http.RoundTripper, error) {
	if base == nil {
		return nil, errors.New("HTTP disk cache requires a base transport")
	}
	if runtime.GOOS == "windows" {
		return nil, errors.New("HTTP disk cache requires Unix file permissions")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, errors.New("HTTP disk cache directory must be a private (0700) directory")
	}
	return &transport{base: base, dir: dir}, nil
}

type transport struct {
	base http.RoundTripper
	dir  string
}

type entry struct {
	Header       http.Header
	Body         []byte
	Uncompressed bool
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet && hasDirective(req.Header, "only-if-cached") {
		// No entry is usable without contacting the origin.
		return &http.Response{
			Status:     "504 Gateway Timeout",
			StatusCode: http.StatusGatewayTimeout,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
	if bypass(req) {
		return t.base.RoundTrip(req)
	}
	// %q retains byte identity even for non-UTF-8 HTTP values. fmt sorts map
	// keys; all explicit headers participate, covering arbitrary Vary fields.
	key := sha256.Sum256(fmt.Appendf(nil, "%q %q %q", req.URL.String(), req.Host, req.Header))
	path := filepath.Join(t.dir, hex.EncodeToString(key[:]))
	saved := load(path)
	if saved != nil {
		conditional := req.Clone(req.Context())
		etag := saved.Header.Get("ETag")
		if conditional.Header == nil {
			conditional.Header = make(http.Header)
		}
		conditional.Header.Set("If-None-Match", etag)
		resp, err := t.base.RoundTrip(conditional)
		if err != nil {
			return resp, err
		}
		if resp.StatusCode != http.StatusNotModified {
			if resp.Request == conditional {
				resp.Request = req
			}
			return storeResponse(path, resp), nil
		}

		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, closeErr
		}
		if confirmed(etag, resp.Header) && !varyUnsupported(resp.Header) {
			return reuse(path, req, resp, saved), nil
		}
		// An unrelated validator or Vary:* cannot confirm this stored body.
		discard(path)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	return storeResponse(path, resp), nil
}

func bypass(req *http.Request) bool {
	if req.Method != http.MethodGet || (req.Body != nil && req.Body != http.NoBody) ||
		hasDirective(req.Header, "no-store") {
		return true
	}
	for name := range req.Header {
		name = strings.ToLower(name)
		if strings.HasPrefix(name, "if-") || name == "range" || name == "upgrade" {
			return true
		}
	}
	return false
}

func headerHas(h http.Header, name string) bool {
	for key := range h {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func hasDirective(h http.Header, name string) bool {
	// Splitting commas in quoted values can only cause a conservative bypass.
	for field, values := range h {
		if !strings.EqualFold(field, "Cache-Control") {
			continue
		}
		for _, line := range values {
			for part := range strings.SplitSeq(line, ",") {
				key, _, _ := strings.Cut(part, "=")
				if strings.EqualFold(strings.TrimSpace(key), name) {
					return true
				}
			}
		}
	}
	return false
}

func varyUnsupported(h http.Header) bool {
	for _, line := range h.Values("Vary") {
		for part := range strings.SplitSeq(line, ",") {
			// The cache itself changes If-None-Match on revalidation.
			switch strings.ToLower(strings.TrimSpace(part)) {
			case "*", "if-none-match", "connection":
				return true
			}
		}
	}
	return false
}

func validETag(h http.Header) bool {
	values := h.Values("ETag")
	if len(values) != 1 {
		return false
	}
	tag := strings.TrimPrefix(values[0], "W/")
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return false
	}
	for i := 1; i < len(tag)-1; i++ {
		// Treat obs-text as unsupported: JSON would replace invalid UTF-8 bytes.
		if b := tag[i]; b <= ' ' || b == '"' || b >= 0x7f {
			return false
		}
	}
	return true
}

func confirmed(etag string, h http.Header) bool {
	return !headerHas(h, "ETag") || (validETag(h) &&
		strings.TrimPrefix(h.Get("ETag"), "W/") == strings.TrimPrefix(etag, "W/"))
}

func cacheable(h http.Header) bool {
	return validETag(h) && !hasDirective(h, "no-store") && !varyUnsupported(h) &&
		!headerHas(h, "Set-Cookie") && !headerHas(h, "Set-Cookie2")
}

// Keep representation and pagination metadata, not request-specific metadata
// such as rate-limit counters, auth state, or cookies.
func representationHeaders(h http.Header) http.Header {
	result := make(http.Header)
	for _, name := range []string{
		"Cache-Control", "Content-Disposition", "Content-Encoding", "Content-Language",
		"Content-Location", "Content-Type", "ETag", "Expires", "Last-Modified", "Link", "Vary",
	} {
		if values := h.Values(name); len(values) > 0 {
			result[http.CanonicalHeaderKey(name)] = slices.Clone(values)
		}
	}
	return result
}

func reuse(path string, req *http.Request, resp *http.Response, saved *entry) *http.Response {
	header := saved.Header.Clone()
	for name, values := range resp.Header {
		switch http.CanonicalHeaderKey(name) {
		case "Content-Length", "Content-Encoding", "Content-Range", "Transfer-Encoding", "Trailer":
			// This response carries the stored body, not the 304's framing.
		default:
			header[name] = values
		}
	}
	persistent := representationHeaders(header)
	if !cacheable(header) {
		discard(path)
	} else if !maps.EqualFunc(saved.Header, persistent, slices.Equal) {
		saved.Header = persistent
		save(path, saved)
	}
	resp.StatusCode = http.StatusOK
	resp.Status = "200 OK"
	resp.Header = header
	resp.Body = io.NopCloser(bytes.NewReader(saved.Body))
	resp.ContentLength = int64(len(saved.Body))
	resp.Uncompressed = saved.Uncompressed
	resp.TransferEncoding = nil
	resp.Trailer = nil
	resp.Request = req
	return resp
}

func storeResponse(path string, resp *http.Response) *http.Response {
	switch resp.StatusCode {
	case http.StatusOK:
		if cacheable(resp.Header) && resp.ContentLength <= maxBodyBytes && len(resp.Trailer) == 0 {
			resp.Body = &recordingBody{
				ReadCloser: resp.Body,
				path:       path,
				length:     resp.ContentLength,
				entry: entry{
					Header:       representationHeaders(resp.Header),
					Uncompressed: resp.Uncompressed,
				},
			}
		} else {
			discard(path)
		}
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		discard(path)
	}
	return resp
}

type recordingBody struct {
	io.ReadCloser
	path   string
	length int64
	entry  entry
	done   bool
}

func (b *recordingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if !b.done {
		if len(b.entry.Body)+n > maxBodyBytes || (err != nil && err != io.EOF) {
			b.done = true
			b.entry.Body = nil
		} else {
			b.entry.Body = append(b.entry.Body, p[:n]...)
			if err == io.EOF {
				b.done = true
				if b.length < 0 || int64(len(b.entry.Body)) == b.length {
					save(b.path, &b.entry)
				}
				b.entry.Body = nil
			}
		}
	}
	return n, err
}

// A disk read failure is a cache miss so it cannot block a live request.
func load(path string) *entry {
	f, err := os.Open(path) //nolint:gosec // Path is the private cache directory plus a hex digest.
	if err != nil {
		logDiskError(err)
		return nil
	}
	defer func() { logDiskError(f.Close()) }()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		logDiskError(err)
		return nil
	}
	if len(data) <= sha256.Size || len(data) > maxFileBytes {
		return nil
	}
	digest := sha256.Sum256(data[sha256.Size:])
	var saved entry
	if !bytes.Equal(data[:sha256.Size], digest[:]) ||
		json.Unmarshal(data[sha256.Size:], &saved) != nil ||
		len(saved.Body) > maxBodyBytes || !cacheable(saved.Header) {
		return nil
	}
	return &saved
}

func save(path string, saved *entry) {
	if err := writeEntry(path, saved); err != nil {
		logDiskError(err)
	}
}

func writeEntry(path string, saved *entry) error {
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if len(data)+sha256.Size > maxFileBytes {
		return nil
	}
	digest := sha256.Sum256(data)
	f, err := os.CreateTemp(filepath.Dir(path), ".entry-")
	if err != nil {
		return err
	}
	defer discard(f.Name())
	_, writeErr := f.Write(append(digest[:], data...))
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func discard(path string) {
	logDiskError(os.Remove(path))
}

func logDiskError(err error) {
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Debug("HTTP disk cache unavailable", "error", err)
	}
}
