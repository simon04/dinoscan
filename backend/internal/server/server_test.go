package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/simon04/dinoscan/backend/internal/petscan"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

func TestStatic(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":       {Data: []byte("<html>app</html>")},
		"assets/app-1.js":  {Data: []byte("js")},
		"favicon.ico":      {Data: []byte("ico")},
		"assets/sub/x.css": {Data: []byte("css")},
	}
	s := New(&petscan.Env{}, dist)
	// Call the handler directly: ServeMux would redirect unclean paths.
	h := http.HandlerFunc(s.handleStatic)
	for _, tt := range []struct {
		path  string
		code  int
		body  string
		cache string
	}{
		{"/", 200, "<html>app</html>", "no-cache"},
		{"/some/route", 200, "<html>app</html>", "no-cache"},
		{"/assets/app-1.js", 200, "js", "public, max-age=31536000, immutable"},
		{"/assets", 200, "<html>app</html>", "no-cache"},
		{"/../../etc/passwd", 400, "invalid URL path\n", ""},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rec.Code != tt.code || rec.Body.String() != tt.body || rec.Header().Get("Cache-Control") != tt.cache {
			t.Errorf("GET %s = %d %q (%s)", tt.path, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
}

func TestAPIErrors(t *testing.T) {
	h := New(&petscan.Env{}, nil).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api?format=json&x=%zz", nil))
	if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), `{"error":"invalid query string`) {
		t.Errorf("bad query = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d", rec.Code)
	}
}

func TestStatusFor(t *testing.T) {
	for err, want := range map[error]int{
		fmt.Errorf("x: %w", petscan.ErrInvalid):  http.StatusBadRequest,
		fmt.Errorf("x: %w", wiki.ErrUnknownWiki): http.StatusBadRequest,
		fmt.Errorf("x: %w", petscan.ErrUpstream): http.StatusBadGateway,
		errors.New("something else"):             http.StatusInternalServerError,
	} {
		if got := statusFor(err); got != want {
			t.Errorf("statusFor(%v) = %d, want %d", err, got, want)
		}
	}
}
