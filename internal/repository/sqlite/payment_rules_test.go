package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"github.com/Goodmoder-ux/collect-bot/migrations"
)

func seedPaymentFixture(t *testing.T) (ctx context.Context, payments *PaymentRepository, studentID, collectionID int64) {
	t.Helper()
	ctx = context.Background()
	db := repositoryTestDB(t)
	result, err := db.Exec(`INSERT INTO students(full_name) VALUES ('Петренко Іван')`)
	if err != nil {
		t.Fatal(err)
	}
	studentID, _ = result.LastInsertId()
	collectionID, err = NewCollectionRepository(db).Create(ctx, "Квіти", 15000, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return ctx, NewPaymentRepository(db), studentID, collectionID
}

func TestUpdateMatchOnlyTouchesUnmatchedPayments(t *testing.T) {
	ctx, payments, studentID, collectionID := seedPaymentFixture(t)
	id, err := payments.Create(ctx, domain.Payment{MonoTxID: "tx-1", Amount: 15000, ReceivedAt: time.Now(), MatchMethod: "unmatched", MatchedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := payments.UpdateMatch(ctx, id, studentID, collectionID, "manual_admin", time.Now()); err != nil {
		t.Fatalf("first match: %v", err)
	}
	err = payments.UpdateMatch(ctx, id, studentID, collectionID, "manual_admin", time.Now())
	if !errors.Is(err, domain.ErrPaymentAlreadyMatched) {
		t.Fatalf("second match: err = %v, want ErrPaymentAlreadyMatched", err)
	}
}

func TestStudentCannotHaveTwoPaymentsForOneCollection(t *testing.T) {
	ctx, payments, studentID, collectionID := seedPaymentFixture(t)
	payment := domain.Payment{
		MonoTxID: "tx-1", Amount: 15000, ReceivedAt: time.Now(), MatchMethod: "auto_exact", MatchedAt: time.Now(),
		StudentID: &studentID, CollectionID: &collectionID,
	}
	if _, err := payments.Create(ctx, payment); err != nil {
		t.Fatal(err)
	}
	payment.MonoTxID = "tx-2"
	if _, err := payments.Create(ctx, payment); err == nil {
		t.Fatal("a second payment for the same student and collection was stored")
	}
	// Unmatched payments are not limited.
	for _, tx := range []string{"tx-3", "tx-4"} {
		if _, err := payments.Create(ctx, domain.Payment{MonoTxID: tx, Amount: 1, ReceivedAt: time.Now(), MatchMethod: "unmatched", MatchedAt: time.Now()}); err != nil {
			t.Fatalf("unmatched payment %s: %v", tx, err)
		}
	}
}

func TestMigrationReturnsDuplicatePaymentsToReview(t *testing.T) {
	const migration = "004_unique_student_collection_payment.sql"
	ctx := context.Background()
	db := repositoryTestDB(t)
	// Rebuild the state of a database created before the migration, with a duplicate in it.
	for _, statement := range []string{
		`DROP INDEX payments_student_collection_unique`,
		`DELETE FROM schema_migrations WHERE version = '` + migration + `'`,
		`INSERT INTO students(id, full_name) VALUES (1, 'Петренко Іван')`,
		`INSERT INTO collections(id, title, amount, deadline_at) VALUES (1, 'Квіти', 15000, '2026-12-01 00:00:00')`,
		`INSERT INTO payments(id, mono_tx_id, amount, received_at, student_id, collection_id, match_method)
			VALUES (1, 'first', 15000, '2026-11-01 10:00:00', 1, 1, 'auto_exact'),
			       (2, 'again', 15000, '2026-11-02 10:00:00', 1, 1, 'auto_exact'),
			       (3, 'other', 500, '2026-11-03 10:00:00', NULL, NULL, 'unmatched')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := applyMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	rows, err := db.Query(`SELECT id, match_method, student_id IS NULL FROM payments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[int64]string{1: "auto_exact", 2: "unmatched", 3: "unmatched"}
	count := 0
	for rows.Next() {
		var id int64
		var method string
		var unassigned bool
		if err := rows.Scan(&id, &method, &unassigned); err != nil {
			t.Fatal(err)
		}
		count++
		if method != want[id] || unassigned != (method == "unmatched") {
			t.Fatalf("payment %d: method=%q unassigned=%v, want method %q", id, method, unassigned, want[id])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("payments = %d, want all 3 kept", count)
	}
}
