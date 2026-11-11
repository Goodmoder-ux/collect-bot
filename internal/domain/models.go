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

type PendingRegistration struct {
	ID          int64
	TelegramID  int64
	StudentID   int64
	Username    string
	RequestedAt time.Time
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
	// GetByTelegramID finds a student linked to a Telegram account.
	GetByTelegramID(ctx context.Context, telegramID int64) (Student, error)
	ListUnregistered(ctx context.Context) ([]Student, error)
	// ListRegistered returns students whose Telegram account has been confirmed.
	ListRegistered(ctx context.Context) ([]Student, error)
	Search(ctx context.Context, query string) ([]Student, error)
	// SetTelegramID links an unregistered student after admin approval.
	SetTelegramID(ctx context.Context, studentID, telegramID int64) error
	UnlinkTelegramID(ctx context.Context, studentID int64) error
	CreatePendingRegistration(ctx context.Context, telegramID, studentID int64, username string) error
	GetPendingRegistrationByStudentID(ctx context.Context, studentID int64) (PendingRegistration, error)
	DeletePendingRegistration(ctx context.Context, studentID int64) error
}

type CollectionRepository interface {
	GetByID(ctx context.Context, id int64) (Collection, error)
	ListActive(ctx context.Context) ([]Collection, error)
	// ListClosed returns up to limit closed collections, most recently closed first.
	ListClosed(ctx context.Context, limit int) ([]Collection, error)
	CloseIfAllStudentsPaid(ctx context.Context, collectionID int64) (bool, error)
	// Create adds an active collection.
	Create(ctx context.Context, title string, amount int64, deadline time.Time) (int64, error)
	// CloseManually closes an active collection by administrator action.
	CloseManually(ctx context.Context, collectionID int64) error
}

type PaymentRepository interface {
	ExistsByMonoTransactionID(ctx context.Context, id string) (bool, error)
	// ExistsForStudentCollection reports whether a payment is assigned to both IDs.
	ExistsForStudentCollection(ctx context.Context, studentID, collectionID int64) (bool, error)
	GetByID(ctx context.Context, id int64) (Payment, error)
	// ListUnmatched returns payments requiring administrator review.
	ListUnmatched(ctx context.Context) ([]Payment, error)
	ListForCollection(ctx context.Context, collectionID int64) ([]Payment, error)
	ListForStudent(ctx context.Context, studentID int64) ([]Payment, error)
	Create(ctx context.Context, payment Payment) error
	UpdateMatch(ctx context.Context, paymentID, studentID, collectionID int64, method string, matchedAt time.Time) error
}

type AdminReportRepository interface {
	// Create persists a report sent to the administrator.
	Create(ctx context.Context, collectionID int64, body string, sentAt time.Time) error
}

type ReminderRepository interface {
	GetLastSent(ctx context.Context, collectionID, studentID int64) (time.Time, error)
	SetLastSent(ctx context.Context, collectionID, studentID int64, sentAt time.Time) error
}

// PollerStateRepository persists the statement polling cursor across restarts.
type PollerStateRepository interface {
	GetLastPoll(ctx context.Context, accountID string) (time.Time, error)
	SetLastPoll(ctx context.Context, accountID string, polledAt time.Time) error
}
