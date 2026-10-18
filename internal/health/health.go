// Package health exposes a localhost-only HTTP health endpoint and a client probe
// used by the container HEALTHCHECK.
package health

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const path = "/healthz"

// Check reports nil when a dependency is healthy.
type Check func(ctx context.Context) error

// Handler runs every check and answers 200 "ok" or 503 with the failures.
func Handler(checks map[string]Check) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		var failures []string
		for name, check := range checks {
			if err := check(ctx); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			}
		}
		if len(failures) > 0 {
			http.Error(w, strings.Join(failures, "\n"), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// Serve listens on 127.0.0.1:port until ctx is cancelled.
func Serve(ctx context.Context, port int, handler http.Handler) error {
	server := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	slog.Info("health endpoint listening", "addr", server.Addr+path)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("health: serve: %w", err)
	}
	return nil
}

// Probe queries the local health endpoint and returns an error unless it is healthy.
func Probe(port int) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + path)
	if err != nil {
		return fmt.Errorf("health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Freshness fails when the last success is older than maxAge. Before the first
// success it allows maxAge from start so a booting service is not reported unhealthy.
func Freshness(lastSuccess func() time.Time, started time.Time, maxAge time.Duration) Check {
	return func(context.Context) error {
		last := lastSuccess()
		if last.IsZero() {
			last = started
		}
		if age := time.Since(last); age > maxAge {
			return fmt.Errorf("no successful run for %s", age.Round(time.Second))
		}
		return nil
	}
}
