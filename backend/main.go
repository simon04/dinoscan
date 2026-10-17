// Command backend serves the DinoScan web frontend and a PetScan-compatible
// query API at /api, reading the Wiki Replicas on Toolforge.
//
// Configuration is by environment variables, see README.md.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/simon04/dinoscan/backend/internal/petscan"
	"github.com/simon04/dinoscan/backend/internal/replica"
	"github.com/simon04/dinoscan/backend/internal/server"
	"github.com/simon04/dinoscan/backend/internal/wiki"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	dbConfig, err := replicaConfig()
	if err != nil {
		return err
	}
	pools := replica.NewPools(dbConfig)
	defer pools.Close()

	maxResults := 1_000_000
	if s := os.Getenv("MAX_RESULTS"); s != "" {
		if maxResults, err = strconv.Atoi(s); err != nil {
			return fmt.Errorf("MAX_RESULTS: %w", err)
		}
	}
	env := &petscan.Env{
		DB:           pools,
		Wiki:         wiki.NewClient(),
		MaxResults:   maxResults,
		UseFileTable: os.Getenv("USE_FILE_TABLE") != "",
		PublicURL:    getenv("PUBLIC_URL", "https://dinoscan.toolforge.org/"),
	}

	dist := getenv("DIST_DIR", "dist")
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		slog.Warn("frontend not found, serving the API only", "dir", dist)
	}
	srv := &http.Server{
		Addr:              ":" + getenv("PORT", "8000"),
		Handler:           server.New(env, os.DirFS(dist)).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// replicaConfig reads the replica credentials from TOOL_REPLICA_USER and
// TOOL_REPLICA_PASSWORD (set by Toolforge), falling back to
// ~/replica.my.cnf, and local tunnel settings from REPLICA_HOST,
// REPLICA_PORT and REPLICA_PORTS (e.g. "enwiki=3306,wikidatawiki=3307").
func replicaConfig() (replica.Config, error) {
	cfg := replica.Config{
		User:     os.Getenv("TOOL_REPLICA_USER"),
		Password: os.Getenv("TOOL_REPLICA_PASSWORD"),
		Host:     os.Getenv("REPLICA_HOST"),
		Ports:    map[string]int{},
	}
	if cfg.User == "" {
		home, _ := os.UserHomeDir()
		user, password, err := readMyCnf(filepath.Join(home, "replica.my.cnf"))
		if err != nil {
			return cfg, fmt.Errorf("no TOOL_REPLICA_USER set and %w", err)
		}
		cfg.User, cfg.Password = user, password
	}
	if s := os.Getenv("REPLICA_PORT"); s != "" {
		port, err := strconv.Atoi(s)
		if err != nil {
			return cfg, fmt.Errorf("REPLICA_PORT: %w", err)
		}
		cfg.Port = port
	}
	for _, pair := range strings.Split(os.Getenv("REPLICA_PORTS"), ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		label, portStr, _ := strings.Cut(pair, "=")
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return cfg, fmt.Errorf("REPLICA_PORTS %q: %w", pair, err)
		}
		cfg.Ports[strings.TrimSpace(label)] = port
	}
	return cfg, nil
}

// readMyCnf reads user and password from a MySQL option file.
func readMyCnf(name string) (string, string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", "", fmt.Errorf("cannot read replica credentials: %w", err)
	}
	defer f.Close()
	var user, password string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `'"`)
		switch strings.TrimSpace(key) {
		case "user":
			user = value
		case "password":
			password = value
		}
	}
	return user, password, scanner.Err()
}
