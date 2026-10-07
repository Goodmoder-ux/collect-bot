package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"collect-bot/internal/domain"
)

type PaymentRepository struct{ db *sql.DB }

func NewPaymentRepository(db *sql.DB) *PaymentRepository { return &PaymentRepository{db: db} }

var _ domain.PaymentRepository = (*PaymentRepository)(nil)

func (r *PaymentRepository) ExistsByMonoTransactionID(ctx context.Context, id string) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payments WHERE mono_tx_id = ?)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check payment %q: %w", id, err)
	}
	return exists, nil
}

func (r *PaymentRepository) ExistsForStudentCollection(ctx context.Context, studentID, collectionID int64) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM payments WHERE student_id = ? AND collection_id = ?
	)`, studentID, collectionID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check payment for student %d and collection %d: %w", studentID, collectionID, err)
	}
	return exists, nil
}

func (r *PaymentRepository) GetByID(ctx context.Context, id int64) (domain.Payment, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, mono_tx_id, amount, comment_raw, received_at, student_id,
		collection_id, match_method, matched_at FROM payments WHERE id = ?`, id)
	payment, err := scanPayment(row)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("get payment %d: %w", id, err)
	}
	return payment, nil
}

func (r *PaymentRepository) Create(ctx context.Context, payment domain.Payment) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO payments
		(mono_tx_id, amount, comment_raw, received_at, student_id, collection_id, match_method, matched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, payment.MonoTxID, payment.Amount, payment.CommentRaw,
		payment.ReceivedAt, nullableID(payment.StudentID), nullableID(payment.CollectionID),
		payment.MatchMethod, payment.MatchedAt)
	if err != nil {
		return fmt.Errorf("create payment %q: %w", payment.MonoTxID, err)
	}
	return nil
}

func (r *PaymentRepository) UpdateMatch(ctx context.Context, paymentID, studentID, collectionID int64, method string, matchedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE payments
		SET student_id = ?, collection_id = ?, match_method = ?, matched_at = ?
		WHERE id = ?`, studentID, collectionID, method, matchedAt, paymentID)
	if err != nil {
		return fmt.Errorf("update payment %d match: %w", paymentID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check payment %d update: %w", paymentID, err)
	}
	if affected != 1 {
		return fmt.Errorf("update payment %d match: %w", paymentID, sql.ErrNoRows)
	}
	return nil
}

func nullableID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

func scanPayment(row rowScanner) (domain.Payment, error) {
	var payment domain.Payment
	var comment sql.NullString
	var studentID, collectionID sql.NullInt64
	if err := row.Scan(&payment.ID, &payment.MonoTxID, &payment.Amount, &comment, &payment.ReceivedAt,
		&studentID, &collectionID, &payment.MatchMethod, &payment.MatchedAt); err != nil {
		return domain.Payment{}, err
	}
	if comment.Valid {
		payment.CommentRaw = comment.String
	}
	if studentID.Valid {
		payment.StudentID = &studentID.Int64
	}
	if collectionID.Valid {
		payment.CollectionID = &collectionID.Int64
	}
	return payment, nil
}
