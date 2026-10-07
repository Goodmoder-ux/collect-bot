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

func (r *StudentRepository) GetByTelegramID(ctx context.Context, telegramID int64) (domain.Student, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, telegram_id, full_name, role, created_at
		FROM students WHERE telegram_id = ?`, telegramID)
	student, err := scanStudent(row)
	if err != nil {
		return domain.Student{}, fmt.Errorf("get student by Telegram ID %d: %w", telegramID, err)
	}
	return student, nil
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
	if err := row.Scan(&student.ID, &telegramID, &student.FullName, &student.Role, &student.CreatedAt); err != nil {
		return domain.Student{}, err
	}
	if telegramID.Valid {
		student.TelegramID = &telegramID.Int64
	}
	return student, nil
}
