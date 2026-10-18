package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

type ReminderRepository struct{ db *sql.DB }

func NewReminderRepository(db *sql.DB) *ReminderRepository { return &ReminderRepository{db: db} }

var _ domain.ReminderRepository = (*ReminderRepository)(nil)

func (r *ReminderRepository) GetLastSent(ctx context.Context, collectionID, studentID int64) (time.Time, error) {
	var sentAt sqliteTime
	err := r.db.QueryRowContext(ctx, `SELECT last_sent_at FROM reminder_state WHERE collection_id = ? AND student_id = ?`,
		collectionID, studentID).Scan(&sentAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("get reminder state for collection %d student %d: %w", collectionID, studentID, err)
	}
	return sentAt.Time, nil
}

func (r *ReminderRepository) SetLastSent(ctx context.Context, collectionID, studentID int64, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO reminder_state(collection_id, student_id, last_sent_at)
		VALUES (?, ?, ?)
		ON CONFLICT(collection_id, student_id) DO UPDATE SET last_sent_at = excluded.last_sent_at`,
		collectionID, studentID, sentAt)
	if err != nil {
		return fmt.Errorf("set reminder state for collection %d student %d: %w", collectionID, studentID, err)
	}
	return nil
}
