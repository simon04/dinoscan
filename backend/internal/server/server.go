// Package server serves the PetScan API and the DinoScan web frontend.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/simon04/dinoscan/backend/internal/petscan"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

const (
	// maxConcurrentQueries bounds the queries running at once; further ones
	// wait up to queueTimeout, then get HTTP 503.
	maxConcurrentQueries = 20
	queueTimeout         = time.Minute
	// queryTimeout bounds the run time of one query.
	queryTimeout = 15 * time.Minute
	// maxBodySize bounds POST bodies, e.g. long manual lists.
	maxBodySize = 16 << 20
)

// Server handles HTTP requests.
type Server struct {
	env   *petscan.Env
	dist  fs.FS // the built frontend; nil to serve none
	slots chan struct{}
}

// New returns a server running queries in env and serving the frontend from
// dist.
func New(env *petscan.Env, dist fs.FS) *Server {
	return &Server{env: env, dist: dist, slots: make(chan struct{}, maxConcurrentQueries)}
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", s.handleAPI)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "OK\n")
	})
	mux.HandleFunc("/", s.handleStatic)
	return mux
}

// handleAPI runs a PetScan query given as URL query or POST form.
func (s *Server) handleAPI(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	var query string
	switch req.Method {
	case http.MethodGet, http.MethodHead:
		query = req.URL.RawQuery
	case http.MethodPost:
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBodySize))
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		query = string(body)
		if req.URL.RawQuery != "" {
			query = req.URL.RawQuery + "&" + query
		}
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	params, err := petscan.ParseParams(query)
	if err != nil {
		writeError(w, "json", http.StatusBadRequest, "invalid query string: "+err.Error())
		return
	}
	format := params.Format()

	ctx := req.Context()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-time.After(queueTimeout):
		writeError(w, format, http.StatusServiceUnavailable, "DinoScan is overloaded, please try again in a few minutes")
		return
	case <-ctx.Done():
		return
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	start := time.Now()
	run := petscan.NewRun(s.env, params)
	err = run.Execute(ctx)
	var resp *petscan.Response
	if err == nil {
		resp, err = run.Render(ctx)
	}
	if err != nil {
		status := statusFor(err)
		slog.Warn("query failed", "status", status, "error", err, "duration", time.Since(start), "query", query)
		writeError(w, format, status, err.Error())
		return
	}
	slog.Info("query", "format", format, "duration", time.Since(start), "bytes", len(resp.Body))
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.Header().Set("Content-Type", resp.ContentType)
	w.WriteHeader(resp.Status)
	if req.Method != http.MethodHead {
		_, _ = w.Write(resp.Body)
	}
}

// statusFor maps an error to an HTTP status.
func statusFor(err error) int {
	switch {
	case errors.Is(err, petscan.ErrInvalid), errors.Is(err, wiki.ErrUnknownWiki):
		return http.StatusBadRequest
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, petscan.ErrUpstream):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// writeError reports an error as {"error": …} for JSON formats, as text
// otherwise.
func writeError(w http.ResponseWriter, format string, status int, message string) {
	if format == "json" || format == "jsonl" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, message+"\n")
}

// handleStatic serves the frontend. Paths that are no file get index.html,
// the single-page app's entry point.
func (s *Server) handleStatic(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dist == nil {
		http.NotFound(w, req)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+req.URL.Path), "/")
	if info, err := fs.Stat(s.dist, name); name == "" || err != nil || info.IsDir() {
		name = "index.html"
	}
	if strings.HasPrefix(name, "assets/") {
		// Vite emits content-hashed asset names.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, req, s.dist, name)
}
