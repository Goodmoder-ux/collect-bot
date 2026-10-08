package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"collect-bot/internal/domain"
	sqliterepository "collect-bot/internal/repository/sqlite"
)

type integrationTransactionSource struct {
	transactions []domain.Transaction
}

func (s integrationTransactionSource) GetTransactions(context.Context, string, time.Time, time.Time) ([]domain.Transaction, error) {
	return s.transactions, nil
}

type integrationFixture struct {
	db          *sql.DB
	students    *sqliterepository.StudentRepository
	collections *sqliterepository.CollectionRepository
	payments    *sqliterepository.PaymentRepository
	matcher     *PaymentMatcher
}

func newIntegrationFixture(t *testing.T) integrationFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "migrations")
	db, err := sqliterepository.Open(context.Background(), ":memory:", migrationsDir)
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	students := sqliterepository.NewStudentRepository(db)
	collections := sqliterepository.NewCollectionRepository(db)
	payments := sqliterepository.NewPaymentRepository(db)
	return integrationFixture{
		db: db, students: students, collections: collections, payments: payments,
		matcher: NewPaymentMatcher(students, collections, payments),
	}
}

func seedStudent(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO students(full_name, role) VALUES (?, 'student')`, name)
	if err != nil {
		t.Fatalf("seed student %q: %v", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get student %q ID: %v", name, err)
	}
	return id
}

func registerStudent(t *testing.T, students *sqliterepository.StudentRepository, studentID, telegramID int64) {
	t.Helper()
	if err := students.SetTelegramID(context.Background(), studentID, telegramID); err != nil {
		t.Fatalf("register student %d: %v", studentID, err)
	}
}

func TestIntegrationHappyPathAutoMatchesAndClosesCollection(t *testing.T) {
	fixture := newIntegrationFixture(t)
	firstID := seedStudent(t, fixture.db, "Петренко Іван")
	secondID := seedStudent(t, fixture.db, "Шевченко Олена")
	registerStudent(t, fixture.students, firstID, 101)
	registerStudent(t, fixture.students, secondID, 102)

	collectionID, err := fixture.collections.Create(
		context.Background(), "Груповий збір", 10000, time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	source := integrationTransactionSource{transactions: []domain.Transaction{
		{ID: "mono-1", Amount: 10000, Comment: "Петренко", OccurredAt: time.Now()},
		{ID: "mono-2", Amount: 10000, Comment: "Шевченко", OccurredAt: time.Now()},
	}}
	transactions, err := source.GetTransactions(context.Background(), "jar", time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("get simulated transactions: %v", err)
	}
	if err := fixture.matcher.Process(context.Background(), transactions); err != nil {
		t.Fatalf("process transactions: %v", err)
	}

	for _, studentID := range []int64{firstID, secondID} {
		paid, err := fixture.payments.ExistsForStudentCollection(context.Background(), studentID, collectionID)
		if err != nil {
			t.Fatalf("check student %d payment: %v", studentID, err)
		}
		if !paid {
			t.Fatalf("student %d payment was not recorded", studentID)
		}
	}
	var matchMethods int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM payments WHERE match_method = 'auto_exact'`).Scan(&matchMethods); err != nil {
		t.Fatalf("count auto matches: %v", err)
	}
	if matchMethods != 2 {
		t.Fatalf("expected 2 auto_exact payments, got %d", matchMethods)
	}

	var status, reason string
	if err := fixture.db.QueryRow(`SELECT status, closed_reason FROM collections WHERE id = ?`, collectionID).Scan(&status, &reason); err != nil {
		t.Fatalf("read collection status: %v", err)
	}
	if status != "closed" || reason != "all_paid" {
		t.Fatalf("expected all-paid collection closure, got status=%q reason=%q", status, reason)
	}
}

func TestIntegrationUnmatchedPaymentCanBeMatchedManually(t *testing.T) {
	fixture := newIntegrationFixture(t)
	studentID := seedStudent(t, fixture.db, "Петренко Іван")
	registerStudent(t, fixture.students, studentID, 101)
	collectionID, err := fixture.collections.Create(
		context.Background(), "Ручне зіставлення", 10000, time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	err = fixture.matcher.Process(context.Background(), []domain.Transaction{{
		ID: "mono-unmatched", Amount: 9999, Comment: "Без відомого студента", OccurredAt: time.Now(),
	}})
	if err != nil {
		t.Fatalf("process unmatched transaction: %v", err)
	}
	unmatched, err := fixture.payments.ListUnmatched(context.Background())
	if err != nil {
		t.Fatalf("list unmatched payments: %v", err)
	}
	if len(unmatched) != 1 {
		t.Fatalf("expected 1 unmatched payment, got %d", len(unmatched))
	}

	matcher := NewPaymentMatcher(fixture.students, fixture.collections, fixture.payments)
	if err := matcher.MatchManually(context.Background(), unmatched[0].ID, studentID, collectionID); err != nil {
		t.Fatalf("manually match payment: %v", err)
	}
	payment, err := fixture.payments.GetByID(context.Background(), unmatched[0].ID)
	if err != nil {
		t.Fatalf("read manually matched payment: %v", err)
	}
	if payment.StudentID == nil || *payment.StudentID != studentID ||
		payment.CollectionID == nil || *payment.CollectionID != collectionID ||
		payment.MatchMethod != "manual_admin" {
		t.Fatalf("unexpected manual match: %#v", payment)
	}
}

func TestIntegrationDuplicateTransactionIsIdempotent(t *testing.T) {
	fixture := newIntegrationFixture(t)
	studentID := seedStudent(t, fixture.db, "Петренко Іван")
	registerStudent(t, fixture.students, studentID, 101)
	_, err := fixture.collections.Create(context.Background(), "Ідемпотентність", 10000, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	transaction := domain.Transaction{
		ID: "mono-duplicate", Amount: 10000, Comment: "Петренко", OccurredAt: time.Now(),
	}

	if err := fixture.matcher.Process(context.Background(), []domain.Transaction{transaction}); err != nil {
		t.Fatalf("process first transaction: %v", err)
	}
	if err := fixture.matcher.Process(context.Background(), []domain.Transaction{transaction}); err != nil {
		t.Fatalf("process duplicate transaction: %v", err)
	}
	var count int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM payments WHERE mono_tx_id = ?`, transaction.ID).Scan(&count); err != nil {
		t.Fatalf("count duplicate transactions: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one payment record, got %d", count)
	}
}
