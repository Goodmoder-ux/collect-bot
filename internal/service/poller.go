package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"collect-bot/internal/domain"
)

type PaymentProcessor interface {
	Process(ctx context.Context, transactions []domain.Transaction) error
}

type Poller struct {
	source    domain.TransactionSource
	processor PaymentProcessor
	accountID string
	interval  time.Duration
	now       func() time.Time

	mu       sync.Mutex
	lastPoll time.Time
}

func NewPoller(source domain.TransactionSource, processor PaymentProcessor, accountID string) *Poller {
	return &Poller{
		source: source, processor: processor, accountID: accountID,
		interval: 70 * time.Second, now: time.Now,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	if err := p.PollOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.PollOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (p *Poller) PollOnce(ctx context.Context) error {
	now := p.now()
	p.mu.Lock()
	from := p.lastPoll
	if from.IsZero() {
		from = now.Add(-2 * time.Minute)
	}
	p.mu.Unlock()

	transactions, err := p.source.GetTransactions(ctx, p.accountID, from, now)
	if err != nil {
		return fmt.Errorf("poll transactions: %w", err)
	}
	if err := p.processor.Process(ctx, transactions); err != nil {
		return fmt.Errorf("process transactions: %w", err)
	}
	p.mu.Lock()
	p.lastPoll = now
	p.mu.Unlock()
	return nil
}
