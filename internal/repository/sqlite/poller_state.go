package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

type PollerStateRepository struct{ db *sql.DB }

func NewPollerStateRepository(db *sql.DB) *PollerStateRepository {
	return &PollerStateRepository{db: db}
}

var _ domain.PollerStateRepository = (*PollerStateRepository)(nil)

func (r *PollerStateRepository) GetLastPoll(ctx context.Context, accountID string) (time.Time, error) {
	var polledAt sqliteTime
	err := r.db.QueryRowContext(ctx, `SELECT last_poll_at FROM poller_state WHERE account_id = ?`, accountID).Scan(&polledAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("get poller state for account %q: %w", accountID, err)
	}
	return polledAt.Time, nil
}

func (r *PollerStateRepository) SetLastPoll(ctx context.Context, accountID string, polledAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO poller_state(account_id, last_poll_at)
		VALUES (?, ?)
		ON CONFLICT(account_id) DO UPDATE SET last_poll_at = excluded.last_poll_at`,
		accountID, polledAt.UTC())
	if err != nil {
		return fmt.Errorf("set poller state for account %q: %w", accountID, err)
	}
	return nil
}
