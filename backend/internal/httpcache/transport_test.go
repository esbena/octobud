package httpcache

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func testDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("disk cache is disabled on Windows")
	}
	return filepath.Join(t.TempDir(), "cache")
}

func testClient(t *testing.T, dir string) *http.Client {
	t.Helper()
	transport, err := New(http.DefaultTransport, dir)
	require.NoError(t, err)
	return &http.Client{Transport: transport}
}

func fetch(
	t *testing.T,
	c *http.Client,
	method, url string,
	header http.Header,
) (resp *http.Response, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, http.NoBody)
	require.NoError(t, err)
	req.Header = header
	resp, err = c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, string(data)
}

func TestRevalidateAndPersist(t *testing.T) {
	dir := testDir(t)
	var version atomic.Int32
	version.Store(1)
	var conditionals atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := strconv.Quote(strconv.Itoa(int(version.Load())))
		w.Header().Set("ETag", tag)
		if got := r.Header.Get("If-None-Match"); got != "" {
			conditionals.Add(1)
			if got == tag {
				w.Header().Set("X-RateLimit-Remaining", "20")
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.Header().Set("Cache-Control", "private, max-age=600")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://example.test/items?page=3>; rel="last"`)
		w.Header().Set("X-RateLimit-Remaining", "21")
		fmt.Fprint(w, tag)
	}))
	defer s.Close()

	const credential = "Bearer fake-credential"
	header := http.Header{"Authorization": {credential}}
	c := testClient(t, dir)
	_, first := fetch(t, c, http.MethodGet, s.URL, header)
	require.Equal(t, `"1"`, first)

	// A new transport can reuse the file, but must first ask the origin.
	c = testClient(t, dir)
	resp, body := fetch(t, c, http.MethodGet, s.URL, header)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, first, body)
	require.Equal(t, int64(len(body)), resp.ContentLength)
	require.Contains(t, resp.Header.Get("Link"), `rel="last"`)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, "20", resp.Header.Get("X-RateLimit-Remaining"))
	require.Empty(t, header.Get("If-None-Match"))

	version.Store(2)
	resp, body = fetch(t, c, http.MethodGet, s.URL, header)
	require.Equal(t, `"2"`, body)
	require.Empty(t, resp.Request.Header.Get("If-None-Match"))
	_, body = fetch(t, c, http.MethodGet, s.URL, header)
	require.Equal(t, `"2"`, body)
	require.EqualValues(t, 3, conditionals.Load())

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	info, err := files[0].Info()
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	path := filepath.Join(dir, files[0].Name())
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), credential)

	require.NoError(t, os.WriteFile(path, raw[:len(raw)-1], 0o600))
	_, body = fetch(t, c, http.MethodGet, s.URL, header)
	require.Equal(t, `"2"`, body)
	require.EqualValues(t, 3, conditionals.Load(), "corrupt file is an unconditional miss")
}

func TestRequestIsolation(t *testing.T) {
	c := testClient(t, testDir(t))
	var conditionals atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"collision"`)
		w.Header().Set("Vary", "X-Variant")
		if r.Header.Get("If-None-Match") != "" {
			conditionals.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprintf(w, "%s|%s|%s|%s|%s|%s",
			r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Cookie"),
			r.Header.Get("Accept"), r.Header.Get("X-GitHub-API-Version"), r.Header.Get("X-Variant"))
	}))
	defer s.Close()

	for _, v := range []struct{ query, auth, cookie, accept, version, variant string }{
		{"page=1", "Bearer a", "sid=a", "json", "v1", "red"},
		{"page=2", "Bearer a", "sid=a", "json", "v1", "red"},
		{"page=1", "Bearer b", "sid=a", "json", "v1", "red"},
		{"page=1", "Bearer a", "sid=b", "json", "v1", "red"},
		{"page=1", "Bearer a", "sid=a", "raw", "v1", "red"},
		{"page=1", "Bearer a", "sid=a", "json", "v2", "red"},
		{"page=1", "Bearer a", "sid=a", "json", "v1", "blue"},
	} {
		header := make(http.Header)
		header.Set("Authorization", v.auth)
		header.Set("Cookie", v.cookie)
		header.Set("Accept", v.accept)
		header.Set("X-GitHub-API-Version", v.version)
		header.Set("X-Variant", v.variant)
		want := fmt.Sprintf(
			"%s|%s|%s|%s|%s|%s", v.query, v.auth, v.cookie, v.accept, v.version, v.variant,
		)
		for range 2 {
			_, body := fetch(t, c, http.MethodGet, s.URL+"/?"+v.query, header)
			require.Equal(t, want, body)
		}
	}
	require.EqualValues(t, 7, conditionals.Load())
}

func TestOnlyIfCachedDoesNotContactOrigin(t *testing.T) {
	c := testClient(t, testDir(t))
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", `"tag"`)
		fmt.Fprint(w, "content")
	}))
	defer s.Close()

	check := func() {
		resp, body := fetch(t, c, http.MethodGet, s.URL, http.Header{
			"Cache-Control": {"only-if-cached"},
		})
		require.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
		require.Empty(t, body)
	}
	check()
	require.Zero(t, requests.Load())
	fetch(t, c, http.MethodGet, s.URL, nil)
	check()
	require.EqualValues(t, 1, requests.Load())
}

func TestBypassAndErrors(t *testing.T) {
	dir := testDir(t)
	c := testClient(t, dir)
	seen := make(chan string, 1)
	var status atomic.Int32
	status.Store(http.StatusOK)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"tag"`)
		switch r.URL.Path {
		case "/no-store":
			w.Header().Add("Cache-Control", "private")
			w.Header().Add("Cache-Control", "NO-STORE")
		case "/no-validator":
			w.Header().Del("ETag")
		case "/vary-star":
			w.Header().Set("Vary", "Accept, *")
		}
		if r.Header.Get("If-None-Match") == `"caller"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(int(status.Load()))
		fmt.Fprint(w, strconv.Itoa(int(status.Load())))
	}))
	defer s.Close()

	for _, path := range []string{"/no-store", "/no-validator", "/vary-star"} {
		for range 2 {
			fetch(t, c, http.MethodGet, s.URL+path, nil)
			require.Empty(t, <-seen)
		}
	}
	for range 2 {
		fetch(t, c, http.MethodPost, s.URL, nil)
		require.Empty(t, <-seen)
		fetch(t, c, http.MethodGet, s.URL, http.Header{"Cache-Control": {"no-store"}})
		require.Empty(t, <-seen)
	}
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)

	fetch(t, c, http.MethodGet, s.URL, nil)
	require.Empty(t, <-seen)
	conditional, _ := fetch(t, c, http.MethodGet, s.URL, http.Header{"If-None-Match": {`"caller"`}})
	require.Equal(t, http.StatusNotModified, conditional.StatusCode)
	require.Equal(t, `"caller"`, <-seen)
	for _, code := range []int{http.StatusBadGateway, http.StatusUnauthorized, http.StatusForbidden} {
		status.Store(int32(code))
		resp, body := fetch(t, c, http.MethodGet, s.URL, nil)
		require.Equal(t, code, resp.StatusCode)
		require.Equal(t, strconv.Itoa(code), body)
		if code == http.StatusForbidden {
			require.Empty(t, <-seen, "401 evicted the prior entry")
		} else {
			require.Equal(t, `"tag"`, <-seen)
		}
	}

	status.Store(http.StatusOK)
	fetch(t, c, http.MethodGet, s.URL, nil)
	<-seen
	s.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestIncompleteAndConcurrentWrites(t *testing.T) {
	dir := testDir(t)
	const body = "complete response"
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"complete"`)
		if r.URL.Path == "/incomplete" {
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "truncated")
			return
		}
		if r.Header.Get("If-None-Match") == `"complete"` {
			hits.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, body)
	}))
	defer s.Close()
	c := testClient(t, dir)
	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, s.URL+"/incomplete", http.NoBody,
	)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.NoError(t, resp.Body.Close())
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)

	t.Run("parallel readers and writers", func(t *testing.T) {
		for i := range 12 {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				for range 3 {
					_, got := fetch(t, c, http.MethodGet, s.URL, nil)
					require.Equal(t, body, got)
				}
			})
		}
	})
	_, got := fetch(t, testClient(t, dir), http.MethodGet, s.URL, nil)
	require.Equal(t, body, got)
	require.Positive(t, hits.Load())
	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
}
