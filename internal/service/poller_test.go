package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	sqliterepository "github.com/Goodmoder-ux/collect-bot/internal/repository/sqlite"
)

type recordingSource struct {
	mu    sync.Mutex
	froms []time.Time
	errs  []error
}

func (s *recordingSource) GetTransactions(_ context.Context, _ string, from, _ time.Time) ([]domain.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.froms = append(s.froms, from)
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		return nil, err
	}
	return nil, nil
}

func (s *recordingSource) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.froms)
}

type noopProcessor struct{}

func (noopProcessor) Process(context.Context, []domain.Transaction) error { return nil }

type fakeRetryAfterError struct{ delay time.Duration }

func (e fakeRetryAfterError) Error() string                  { return "rate limited" }
func (e fakeRetryAfterError) RetryAfterDelay() time.Duration { return e.delay }

func TestPollerCursorSurvivesRestart(t *testing.T) {
	fixture := newIntegrationFixture(t)
	state := sqliterepository.NewPollerStateRepository(fixture.db)
	firstPoll := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	source := &recordingSource{}
	first := NewPoller(source, noopProcessor{}, state, "jar", time.Minute)
	first.now = func() time.Time { return firstPoll }
	if err := first.PollOnce(context.Background()); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	if want := firstPoll.Add(-firstPollLookback); !source.froms[0].Equal(want) {
		t.Fatalf("first poll from = %v, want %v", source.froms[0], want)
	}

	// A new poller simulates a process restart three hours later.
	restarted := NewPoller(source, noopProcessor{}, state, "jar", time.Minute)
	restarted.now = func() time.Time { return firstPoll.Add(3 * time.Hour) }
	if err := restarted.PollOnce(context.Background()); err != nil {
		t.Fatalf("poll after restart: %v", err)
	}
	if want := firstPoll.Add(-pollOverlap); !source.froms[1].Equal(want) {
		t.Fatalf("poll after restart from = %v, want %v", source.froms[1], want)
	}
}

func TestPollerClampsFromToStatementWindow(t *testing.T) {
	fixture := newIntegrationFixture(t)
	state := sqliterepository.NewPollerStateRepository(fixture.db)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := state.SetLastPoll(context.Background(), "jar", now.Add(-90*24*time.Hour)); err != nil {
		t.Fatalf("seed poll cursor: %v", err)
	}

	source := &recordingSource{}
	poller := NewPoller(source, noopProcessor{}, state, "jar", time.Minute)
	poller.now = func() time.Time { return now }
	if err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if want := now.Add(-maxStatementWindow); !source.froms[0].Equal(want) {
		t.Fatalf("from = %v, want %v", source.froms[0], want)
	}
}

func TestPollerRunKeepsGoingAfterErrors(t *testing.T) {
	fixture := newIntegrationFixture(t)
	state := sqliterepository.NewPollerStateRepository(fixture.db)
	source := &recordingSource{errs: []error{errors.New("boom"), fakeRetryAfterError{}}}
	poller := NewPoller(source, noopProcessor{}, state, "jar", time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- poller.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for poller.LastSuccess().IsZero() {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("no successful poll after %d calls", source.calls())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled", err)
	}
	if source.calls() < 3 {
		t.Fatalf("poller made %d calls, want at least 3", source.calls())
	}
}

func TestPollerRetryDelay(t *testing.T) {
	poller := NewPoller(nil, nil, nil, "jar", time.Minute)
	tests := []struct {
		name     string
		err      error
		failures int
		want     time.Duration
	}{
		{"first failure waits one interval", errors.New("x"), 1, time.Minute},
		{"backoff doubles", errors.New("x"), 2, 2 * time.Minute},
		{"backoff is capped", errors.New("x"), 30, maxRetryDelay},
		{"server retry-after wins when longer", fakeRetryAfterError{delay: 10 * time.Minute}, 1, 10 * time.Minute},
		{"short retry-after does not shorten backoff", fakeRetryAfterError{delay: time.Second}, 1, time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := poller.retryDelay(tt.err, tt.failures); got != tt.want {
				t.Fatalf("retryDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}
