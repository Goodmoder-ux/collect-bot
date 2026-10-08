package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"collect-bot/internal/domain"
)

type StudentRepository struct{ db *sql.DB }

func NewStudentRepository(db *sql.DB) *StudentRepository { return &StudentRepository{db: db} }

var _ domain.StudentRepository = (*StudentRepository)(nil)

func (r *StudentRepository) GetByID(ctx context.Context, id int64) (domain.Student, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, telegram_id, full_name, role, created_at FROM students WHERE id = ?`, id)
	student, err := scanStudent(row)
	if err != nil {
		return domain.Student{}, fmt.Errorf("get student %d: %w", id, err)
	}
	return student, nil
}

// GetByTelegramID returns the student linked to telegramID.
func (r *StudentRepository) GetByTelegramID(ctx context.Context, telegramID int64) (domain.Student, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, telegram_id, full_name, role, created_at
		FROM students WHERE telegram_id = ?`, telegramID)
	student, err := scanStudent(row)
	if err != nil {
		return domain.Student{}, fmt.Errorf("get student by Telegram ID %d: %w", telegramID, err)
	}
	return student, nil
}

func (r *StudentRepository) SetTelegramID(ctx context.Context, studentID, telegramID int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE students SET telegram_id = ? WHERE id = ? AND telegram_id IS NULL`,
		telegramID, studentID)
	if err != nil {
		return fmt.Errorf("link student %d to Telegram ID %d: %w", studentID, telegramID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check student %d link: %w", studentID, err)
	}
	if affected != 1 {
		return fmt.Errorf("student %d is already linked or does not exist", studentID)
	}
	return nil
}

func (r *StudentRepository) CreatePendingRegistration(ctx context.Context, telegramID, studentID int64, username string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pending registration for Telegram ID %d: %w", telegramID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pending_registrations WHERE student_id = ?`, studentID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("replace pending registration for student %d: %w", studentID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pending_registrations(telegram_id, student_id, username)
		VALUES (?, ?, ?)
		ON CONFLICT(telegram_id) DO UPDATE SET student_id = excluded.student_id,
		username = excluded.username, requested_at = CURRENT_TIMESTAMP`,
		telegramID, studentID, nullableString(username)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("create pending registration for Telegram ID %d: %w", telegramID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pending registration for Telegram ID %d: %w", telegramID, err)
	}
	return nil
}

func (r *StudentRepository) GetPendingRegistrationByStudentID(ctx context.Context, studentID int64) (domain.PendingRegistration, error) {
	var pending domain.PendingRegistration
	var username sql.NullString
	var requestedAt sqliteTime
	err := r.db.QueryRowContext(ctx, `SELECT id, telegram_id, student_id, username, requested_at
		FROM pending_registrations WHERE student_id = ?`, studentID).
		Scan(&pending.ID, &pending.TelegramID, &pending.StudentID, &username, &requestedAt)
	if err != nil {
		return domain.PendingRegistration{}, fmt.Errorf("get pending registration for student %d: %w", studentID, err)
	}
	pending.Username = username.String
	pending.RequestedAt = requestedAt.Time
	return pending, nil
}

func (r *StudentRepository) DeletePendingRegistration(ctx context.Context, studentID int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM pending_registrations WHERE student_id = ?`, studentID); err != nil {
		return fmt.Errorf("delete pending registration for student %d: %w", studentID, err)
	}
	return nil
}

func (r *StudentRepository) ListUnregistered(ctx context.Context) ([]domain.Student, error) {
	return r.list(ctx, `WHERE telegram_id IS NULL`)
}

func (r *StudentRepository) ListRegistered(ctx context.Context) ([]domain.Student, error) {
	return r.list(ctx, `WHERE telegram_id IS NOT NULL`)
}

func (r *StudentRepository) list(ctx context.Context, condition string) ([]domain.Student, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, telegram_id, full_name, role, created_at FROM students `+condition+` ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list students: %w", err)
	}
	defer rows.Close()
	var result []domain.Student
	for rows.Next() {
		student, err := scanStudent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}
		result = append(result, student)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate students: %w", err)
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanStudent(row rowScanner) (domain.Student, error) {
	var student domain.Student
	var telegramID sql.NullInt64
	var createdAt sqliteTime
	if err := row.Scan(&student.ID, &telegramID, &student.FullName, &student.Role, &createdAt); err != nil {
		return domain.Student{}, err
	}
	student.CreatedAt = createdAt.Time
	if telegramID.Valid {
		student.TelegramID = &telegramID.Int64
	}
	return student, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
