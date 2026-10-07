package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"collect-bot/internal/domain"
)

type AdminReportRepository struct{ db *sql.DB }

func NewAdminReportRepository(db *sql.DB) *AdminReportRepository {
	return &AdminReportRepository{db: db}
}

var _ domain.AdminReportRepository = (*AdminReportRepository)(nil)

func (r *AdminReportRepository) Create(ctx context.Context, collectionID int64, body string, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO admin_reports(collection_id, body, sent_at) VALUES (?, ?, ?)`,
		collectionID, body, sentAt)
	if err != nil {
		return fmt.Errorf("create admin report for collection %d: %w", collectionID, err)
	}
	return nil
}
