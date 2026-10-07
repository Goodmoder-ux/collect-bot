package domain

import (
	"context"
	"time"
)

type Student struct {
	ID         int64
	TelegramID *int64
	FullName   string
	Role       string
	CreatedAt  time.Time
}

type Collection struct {
	ID           int64
	Title        string
	Amount       int64
	DeadlineAt   time.Time
	Status       string
	ClosedReason *string
	CreatedAt    time.Time
	ClosedAt     *time.Time
}

type Payment struct {
	ID           int64
	MonoTxID     string
	Amount       int64
	CommentRaw   string
	ReceivedAt   time.Time
	StudentID    *int64
	CollectionID *int64
	MatchMethod  string
	MatchedAt    time.Time
}

type Transaction struct {
	ID         string
	Amount     int64
	Comment    string
	OccurredAt time.Time
}

type TransactionSource interface {
	GetTransactions(ctx context.Context, accountID string, from, to time.Time) ([]Transaction, error)
}

type StudentRepository interface {
	GetByID(ctx context.Context, id int64) (Student, error)
	ListUnregistered(ctx context.Context) ([]Student, error)
}

type CollectionRepository interface {
	ListActive(ctx context.Context) ([]Collection, error)
}

type PaymentRepository interface {
	ExistsByMonoTransactionID(ctx context.Context, id string) (bool, error)
	Create(ctx context.Context, payment Payment) error
}

type ReminderRepository interface {
	GetLastSent(ctx context.Context, collectionID, studentID int64) (time.Time, error)
	SetLastSent(ctx context.Context, collectionID, studentID int64, sentAt time.Time) error
}
