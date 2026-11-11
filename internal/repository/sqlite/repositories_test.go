package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

func repositoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), ":memory:")
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
	if got, err := students.Search(ctx, "Петренко"); err != nil || len(got) != 1 {
		t.Fatalf("search students: got %#v, err %v", got, err)
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
	if _, err := payments.Create(ctx, domain.Payment{
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
	if active, err := collections.ListActive(ctx); err != nil || len(active) != 0 {
		t.Fatalf("active after close = %v, %v; want none", active, err)
	}
	closed, err := collections.ListClosed(ctx, 10)
	if err != nil || len(closed) != 1 {
		t.Fatalf("closed = %v, %v; want one collection", closed, err)
	}
	if closed[0].ID != collectionID || closed[0].ClosedAt == nil || closed[0].ClosedReason == nil || *closed[0].ClosedReason != "all_paid" {
		t.Fatalf("closed collection = %+v", closed[0])
	}
	if limited, err := collections.ListClosed(ctx, 0); err != nil || len(limited) != 0 {
		t.Fatalf("closed with limit 0 = %v, %v", limited, err)
	}
}

func TestStudentRepositoryUnlinksTelegramAccount(t *testing.T) {
	ctx := context.Background()
	db := repositoryTestDB(t)
	repository := NewStudentRepository(db)
	result, err := db.Exec(`INSERT INTO students(full_name, telegram_id) VALUES ('Коваленко Ірина', 99)`)
	if err != nil {
		t.Fatal(err)
	}
	studentID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UnlinkTelegramID(ctx, studentID); err != nil {
		t.Fatalf("unlink student: %v", err)
	}
	if registered, err := repository.ListRegistered(ctx); err != nil || len(registered) != 0 {
		t.Fatalf("registered students after unlink: %v, count=%d", err, len(registered))
	}
}

func TestCollectionRepositoryReadsSQLiteTextTimestamps(t *testing.T) {
	ctx := context.Background()
	db := repositoryTestDB(t)
	repository := NewCollectionRepository(db)

	_, err := db.Exec(`INSERT INTO collections
		(title, amount, deadline_at, status, created_at)
		VALUES (?, ?, ?, 'active', ?)`,
		"Текстова дата", 10000, "2026-10-08 12:00:00", "2026-10-08 10:00:00")
	if err != nil {
		t.Fatalf("insert text timestamps: %v", err)
	}

	collections, err := repository.ListActive(ctx)
	if err != nil {
		t.Fatalf("list collection with text timestamps: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("expected one collection, got %d", len(collections))
	}
	if collections[0].DeadlineAt.IsZero() || collections[0].CreatedAt.IsZero() {
		t.Fatalf("expected parsed timestamps, got deadline=%v created=%v",
			collections[0].DeadlineAt, collections[0].CreatedAt)
	}
}

func TestCollectionRepositoryReadsSQLiteGoTimestamp(t *testing.T) {
	ctx := context.Background()
	db := repositoryTestDB(t)
	repository := NewCollectionRepository(db)

	_, err := db.Exec(`INSERT INTO collections
		(title, amount, deadline_at, status, created_at)
		VALUES (?, ?, ?, 'active', ?)`,
		"Go timestamp", 10000, "2026-12-31 23:59:00 +0300 +0300", "2026-10-08 10:00:00")
	if err != nil {
		t.Fatalf("insert Go timestamp: %v", err)
	}

	collections, err := repository.ListActive(ctx)
	if err != nil {
		t.Fatalf("list collection with Go timestamp: %v", err)
	}
	if len(collections) != 1 || collections[0].DeadlineAt.IsZero() {
		t.Fatalf("expected parsed Go timestamp, got %#v", collections)
	}
}

func TestStudentRepositoryPersistsPendingRegistration(t *testing.T) {
	ctx := context.Background()
	db := repositoryTestDB(t)
	repository := NewStudentRepository(db)

	result, err := db.Exec(`INSERT INTO students(full_name, role) VALUES ('Шевченко Олена', 'student')`)
	if err != nil {
		t.Fatalf("insert student: %v", err)
	}
	studentID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get student ID: %v", err)
	}

	if err := repository.CreatePendingRegistration(ctx, 12345, studentID, "olena"); err != nil {
		t.Fatalf("create pending registration: %v", err)
	}
	pending, err := repository.GetPendingRegistrationByStudentID(ctx, studentID)
	if err != nil {
		t.Fatalf("get pending registration: %v", err)
	}
	if pending.TelegramID != 12345 || pending.StudentID != studentID || pending.Username != "olena" {
		t.Fatalf("unexpected pending registration: %#v", pending)
	}
	if err := repository.DeletePendingRegistration(ctx, studentID); err != nil {
		t.Fatalf("delete pending registration: %v", err)
	}
	if _, err := repository.GetPendingRegistrationByStudentID(ctx, studentID); err == nil {
		t.Fatal("expected deleted pending registration to be unavailable")
	}
}
