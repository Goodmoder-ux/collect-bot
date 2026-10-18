package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

const (
	// DefaultPollInterval respects Monobank's limit of one statement request per minute.
	DefaultPollInterval = 70 * time.Second
	// maxStatementWindow is the longest range Monobank accepts in one statement request.
	maxStatementWindow = 31*24*time.Hour + time.Hour
	// pollOverlap re-reads a short window to catch transactions the bank posts late.
	// Already stored transactions are skipped by the matcher.
	pollOverlap       = 5 * time.Minute
	firstPollLookback = 2 * time.Minute
	maxRetryDelay     = 5 * time.Minute
)

type PaymentProcessor interface {
	Process(ctx context.Context, transactions []domain.Transaction) error
}

// retryAfterError is implemented by errors that carry a server-requested retry delay.
type retryAfterError interface {
	RetryAfterDelay() time.Duration
}

type Poller struct {
	source    domain.TransactionSource
	processor PaymentProcessor
	state     domain.PollerStateRepository
	accountID string
	interval  time.Duration
	now       func() time.Time

	mu          sync.Mutex
	lastPoll    time.Time
	lastSuccess time.Time
}

func NewPoller(source domain.TransactionSource, processor PaymentProcessor, state domain.PollerStateRepository, accountID string, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	return &Poller{
		source: source, processor: processor, state: state, accountID: accountID,
		interval: interval, now: time.Now,
	}
}

// Run polls until ctx is cancelled. Failures are logged and retried with backoff.
func (p *Poller) Run(ctx context.Context) error {
	var delay time.Duration
	failures := 0
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if err := p.PollOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures++
			delay = p.retryDelay(err, failures)
			slog.Warn("monobank poll failed", "err", err, "failures", failures, "retry_in", delay)
			continue
		}
		if failures > 0 {
			slog.Info("monobank poll recovered", "failures", failures)
		}
		failures = 0
		delay = p.interval
	}
}

func (p *Poller) retryDelay(err error, failures int) time.Duration {
	delay := maxRetryDelay
	if failures < 8 {
		delay = min(p.interval<<(failures-1), maxRetryDelay)
	}
	var retry retryAfterError
	if errors.As(err, &retry) && retry.RetryAfterDelay() > delay {
		delay = retry.RetryAfterDelay()
	}
	return delay
}

func (p *Poller) PollOnce(ctx context.Context) error {
	now := p.now()
	from, err := p.cursor(ctx, now)
	if err != nil {
		return err
	}
	if oldest := now.Add(-maxStatementWindow); from.Before(oldest) {
		slog.Warn("poll gap exceeds Monobank statement window; older transactions are skipped",
			"last_poll", from, "window_start", oldest)
		from = oldest
	}

	transactions, err := p.source.GetTransactions(ctx, p.accountID, from, now)
	if err != nil {
		return fmt.Errorf("poll transactions: %w", err)
	}
	slog.Debug("monobank poll", "from", from, "to", now, "transactions", len(transactions))
	if err := p.processor.Process(ctx, transactions); err != nil {
		return fmt.Errorf("process transactions: %w", err)
	}
	if err := p.state.SetLastPoll(ctx, p.accountID, now); err != nil {
		return fmt.Errorf("save poll cursor: %w", err)
	}
	p.mu.Lock()
	p.lastPoll = now
	p.lastSuccess = p.now()
	p.mu.Unlock()
	return nil
}

// LastSuccess reports when the last poll completed without error.
func (p *Poller) LastSuccess() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastSuccess
}

func (p *Poller) cursor(ctx context.Context, now time.Time) (time.Time, error) {
	p.mu.Lock()
	lastPoll := p.lastPoll
	p.mu.Unlock()
	if lastPoll.IsZero() {
		stored, err := p.state.GetLastPoll(ctx, p.accountID)
		if err != nil {
			return time.Time{}, fmt.Errorf("load poll cursor: %w", err)
		}
		lastPoll = stored
	}
	if lastPoll.IsZero() {
		return now.Add(-firstPollLookback), nil
	}
	return lastPoll.Add(-pollOverlap), nil
}
