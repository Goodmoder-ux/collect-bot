package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

type CollectionRepository struct{ db *sql.DB }

func NewCollectionRepository(db *sql.DB) *CollectionRepository { return &CollectionRepository{db: db} }

var _ domain.CollectionRepository = (*CollectionRepository)(nil)

func (r *CollectionRepository) GetByID(ctx context.Context, id int64) (domain.Collection, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, title, amount, deadline_at, status, closed_reason, created_at, closed_at
		FROM collections WHERE id = ?`, id)
	collection, err := scanCollection(row)
	if err != nil {
		return domain.Collection{}, fmt.Errorf("get collection %d: %w", id, err)
	}
	return collection, nil
}

func (r *CollectionRepository) ListActive(ctx context.Context) ([]domain.Collection, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, title, amount, deadline_at, status, closed_reason, created_at, closed_at
		FROM collections WHERE status = 'active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list active collections: %w", err)
	}
	defer rows.Close()
	var result []domain.Collection
	for rows.Next() {
		collection, err := scanCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		result = append(result, collection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collections: %w", err)
	}
	return result, nil
}

func (r *CollectionRepository) CloseIfAllStudentsPaid(ctx context.Context, collectionID int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE collections
		SET status = 'closed', closed_reason = 'all_paid', closed_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'active'
		AND NOT EXISTS (
			SELECT 1 FROM students s
			WHERE s.telegram_id IS NOT NULL
			AND NOT EXISTS (
				SELECT 1 FROM payments p
				WHERE p.collection_id = collections.id AND p.student_id = s.id
			)
		)
		AND EXISTS (SELECT 1 FROM students WHERE telegram_id IS NOT NULL)`, collectionID)
	if err != nil {
		return false, fmt.Errorf("close collection %d when all students paid: %w", collectionID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check collection %d close result: %w", collectionID, err)
	}
	return count == 1, nil
}

func scanCollection(row rowScanner) (domain.Collection, error) {
	var collection domain.Collection
	var closedReason sql.NullString
	var deadlineAt, createdAt, closedAt sqliteTime
	if err := row.Scan(&collection.ID, &collection.Title, &collection.Amount, &deadlineAt,
		&collection.Status, &closedReason, &createdAt, &closedAt); err != nil {
		return domain.Collection{}, err
	}
	collection.DeadlineAt = deadlineAt.Time
	collection.CreatedAt = createdAt.Time
	if closedReason.Valid {
		collection.ClosedReason = &closedReason.String
	}
	if closedAt.valid {
		closed := closedAt.Time
		collection.ClosedAt = &closed
	}
	return collection, nil
}

func (r *CollectionRepository) Create(ctx context.Context, title string, amount int64, deadline time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO collections(title, amount, deadline_at) VALUES (?, ?, ?)`, title, amount, deadline)
	if err != nil {
		return 0, fmt.Errorf("create collection: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get created collection ID: %w", err)
	}
	return id, nil
}

// CloseManually closes an active collection by administrator request.
func (r *CollectionRepository) CloseManually(ctx context.Context, collectionID int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE collections
		SET status = 'closed', closed_reason = 'manual_admin', closed_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'active'`, collectionID)
	if err != nil {
		return fmt.Errorf("close collection %d manually: %w", collectionID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check manual close for collection %d: %w", collectionID, err)
	}
	if affected != 1 {
		return fmt.Errorf("collection %d is not active", collectionID)
	}
	return nil
}
