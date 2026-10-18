package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		name   string
		checks map[string]Check
		want   int
	}{
		{"healthy", map[string]Check{"db": func(context.Context) error { return nil }}, http.StatusOK},
		{"failing check", map[string]Check{"db": func(context.Context) error { return errors.New("down") }}, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Handler(tt.checks).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestFreshness(t *testing.T) {
	never := func() time.Time { return time.Time{} }
	recent := func() time.Time { return time.Now().Add(-time.Minute) }
	stale := func() time.Time { return time.Now().Add(-time.Hour) }

	if err := Freshness(never, time.Now(), 5*time.Minute)(context.Background()); err != nil {
		t.Fatalf("booting service reported unhealthy: %v", err)
	}
	if err := Freshness(never, time.Now().Add(-time.Hour), 5*time.Minute)(context.Background()); err == nil {
		t.Fatal("service that never succeeded reported healthy after the grace period")
	}
	if err := Freshness(recent, time.Now().Add(-time.Hour), 5*time.Minute)(context.Background()); err != nil {
		t.Fatalf("recent success reported unhealthy: %v", err)
	}
	if err := Freshness(stale, time.Now().Add(-time.Hour), 5*time.Minute)(context.Background()); err == nil {
		t.Fatal("stale success reported healthy")
	}
}
