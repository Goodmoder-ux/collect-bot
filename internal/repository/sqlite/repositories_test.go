package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"collect-bot/internal/domain"
)

func repositoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	db, err := Open(context.Background(), ":memory:", filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRepositoriesCoverCorePersistence(t *testing.T) {
	ctx := context.Background()
	db := repositoryTestDB(t)
	students := NewStudentRepository(db)
	collections := NewCollectionRepository(db)
	payments := NewPaymentRepository(db)
	reminders := NewReminderRepository(db)
	reports := NewAdminReportRepository(db)

	studentID, err := db.Exec(`INSERT INTO students(full_name, role) VALUES ('Петренко Іван', 'student')`)
	if err != nil {
		t.Fatal(err)
	}
	studentIDValue, err := studentID.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	telegramID := int64(42)
	if err := students.SetTelegramID(ctx, studentIDValue, telegramID); err != nil {
		t.Fatal(err)
	}
	if _, err := students.GetByID(ctx, studentIDValue); err != nil {
		t.Fatal(err)
	}
	if _, err := students.GetByTelegramID(ctx, telegramID); err != nil {
		t.Fatal(err)
	}
	if registered, err := students.ListRegistered(ctx); err != nil || len(registered) != 1 {
		t.Fatalf("registered students: %v, count=%d", err, len(registered))
	}
	if unregistered, err := students.ListUnregistered(ctx); err != nil || len(unregistered) != 0 {
		t.Fatalf("unregistered students: %v, count=%d", err, len(unregistered))
	}

	collectionID, err := collections.Create(ctx, "Збір", 10000, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if active, err := collections.ListActive(ctx); err != nil || len(active) != 1 {
		t.Fatalf("active collections: %v, count=%d", err, len(active))
	}
	if last, err := reminders.GetLastSent(ctx, collectionID, studentIDValue); err != nil || !last.IsZero() {
		t.Fatalf("initial reminder state: %v, time=%v", err, last)
	}
	reminderAt := time.Now().UTC()
	if err := reminders.SetLastSent(ctx, collectionID, studentIDValue, reminderAt); err != nil {
		t.Fatal(err)
	}
	if last, err := reminders.GetLastSent(ctx, collectionID, studentIDValue); err != nil || last.IsZero() {
		t.Fatalf("saved reminder state: %v, time=%v", err, last)
	}

	receivedAt := time.Now().UTC()
	if err := payments.Create(ctx, domain.Payment{
		MonoTxID: "mono-repository", Amount: 10000, CommentRaw: "Петренко",
		ReceivedAt: receivedAt, StudentID: &studentIDValue, CollectionID: &collectionID,
		MatchMethod: "auto_exact", MatchedAt: receivedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if exists, err := payments.ExistsByMonoTransactionID(ctx, "mono-repository"); err != nil || !exists {
		t.Fatalf("transaction existence: %v, exists=%v", err, exists)
	}
	if exists, err := payments.ExistsForStudentCollection(ctx, studentIDValue, collectionID); err != nil || !exists {
		t.Fatalf("student collection payment: %v, exists=%v", err, exists)
	}
	var paymentID int64
	if err := db.QueryRow(`SELECT id FROM payments WHERE mono_tx_id = 'mono-repository'`).Scan(&paymentID); err != nil {
		t.Fatal(err)
	}
	if _, err := payments.GetByID(ctx, paymentID); err != nil {
		t.Fatal(err)
	}
	if err := reports.Create(ctx, collectionID, "Звіт", receivedAt); err != nil {
		t.Fatal(err)
	}
	if closed, err := collections.CloseIfAllStudentsPaid(ctx, collectionID); err != nil || !closed {
		t.Fatalf("auto close: %v, closed=%v", err, closed)
	}
	if err := collections.CloseManually(ctx, collectionID); err == nil {
		t.Fatal("expected manual close of closed collection to fail")
	}
}
