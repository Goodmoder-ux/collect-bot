package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"collect-bot/internal/domain"
)

type fakeStudents struct {
	students []domain.Student
}

func (f *fakeStudents) GetByID(_ context.Context, id int64) (domain.Student, error) {
	for _, student := range f.students {
		if student.ID == id {
			return student, nil
		}
	}
	return domain.Student{}, errors.New("student not found")
}

func (f *fakeStudents) GetByTelegramID(_ context.Context, telegramID int64) (domain.Student, error) {
	for _, student := range f.students {
		if student.TelegramID != nil && *student.TelegramID == telegramID {
			return student, nil
		}
	}
	return domain.Student{}, errors.New("student not found")
}

func (f *fakeStudents) SetTelegramID(context.Context, int64, int64) error { return nil }

func (f *fakeStudents) ListUnregistered(context.Context) ([]domain.Student, error) { return nil, nil }
func (f *fakeStudents) ListRegistered(context.Context) ([]domain.Student, error) {
	return f.students, nil
}

type fakeCollections struct {
	collections []domain.Collection
	closed      []int64
}

func (f *fakeCollections) ListActive(context.Context) ([]domain.Collection, error) {
	return f.collections, nil
}
func (f *fakeCollections) CloseIfAllStudentsPaid(_ context.Context, id int64) (bool, error) {
	f.closed = append(f.closed, id)
	return false, nil
}
func (f *fakeCollections) Create(context.Context, string, int64, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeCollections) CloseManually(context.Context, int64) error { return nil }

type fakePayments struct {
	existing map[string]bool
	created  []domain.Payment
	updated  []int64
}

func (f *fakePayments) ExistsByMonoTransactionID(_ context.Context, id string) (bool, error) {
	return f.existing[id], nil
}
func (f *fakePayments) ExistsForStudentCollection(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (f *fakePayments) GetByID(context.Context, int64) (domain.Payment, error) {
	return domain.Payment{}, errors.New("not implemented")
}
func (f *fakePayments) ListUnmatched(context.Context) ([]domain.Payment, error) { return nil, nil }
func (f *fakePayments) Create(_ context.Context, payment domain.Payment) error {
	f.created = append(f.created, payment)
	return nil
}
func (f *fakePayments) UpdateMatch(_ context.Context, paymentID, _, _ int64, _ string, _ time.Time) error {
	f.updated = append(f.updated, paymentID)
	return nil
}

func newMatcher(students []domain.Student, collections []domain.Collection, payments *fakePayments) *PaymentMatcher {
	matcher := NewPaymentMatcher(&fakeStudents{students: students}, &fakeCollections{collections: collections}, payments)
	matcher.now = func() time.Time { return time.Unix(100, 0) }
	return matcher
}

func testStudent(id int64, name string) domain.Student {
	return domain.Student{ID: id, FullName: name, TelegramID: pointer(id)}
}

func testCollection(id, amount int64) domain.Collection {
	return domain.Collection{ID: id, Amount: amount, Status: "active"}
}

func pointer(value int64) *int64 { return &value }

func TestPaymentMatcherExactName(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{}}
	matcher := newMatcher([]domain.Student{testStudent(1, "Петренко Іван")}, []domain.Collection{testCollection(2, 15000)}, payments)

	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-1", Amount: 15000, Comment: "Оплата Петренко Іван", OccurredAt: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(payments.created) != 1 || payments.created[0].MatchMethod != "auto_exact" {
		t.Fatalf("expected one auto_exact payment, got %#v", payments.created)
	}
	if *payments.created[0].StudentID != 1 || *payments.created[0].CollectionID != 2 {
		t.Fatalf("unexpected match: %#v", payments.created[0])
	}
}

func TestPaymentMatcherAcceptsSurnameVariation(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{}}
	matcher := newMatcher([]domain.Student{testStudent(1, "Петренко Іван")}, []domain.Collection{testCollection(2, 15000)}, payments)

	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-2", Amount: 15000, Comment: "за Петренка", OccurredAt: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := payments.created[0].MatchMethod; got != "auto_exact" {
		t.Fatalf("expected auto_exact, got %q", got)
	}
}

func TestPaymentMatcherMarksMultipleCandidatesUnmatched(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{}}
	matcher := newMatcher([]domain.Student{
		testStudent(1, "Петренко Іван"),
		testStudent(2, "Петренко Олена"),
	}, []domain.Collection{testCollection(2, 15000)}, payments)

	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-3", Amount: 15000, Comment: "Петренко", OccurredAt: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := payments.created[0].MatchMethod; got != "unmatched" {
		t.Fatalf("expected unmatched, got %q", got)
	}
	if payments.created[0].StudentID != nil || payments.created[0].CollectionID != nil {
		t.Fatalf("unmatched payment should have no assignments: %#v", payments.created[0])
	}
}

func TestPaymentMatcherMarksWrongAmountUnmatched(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{}}
	matcher := newMatcher([]domain.Student{testStudent(1, "Петренко Іван")}, []domain.Collection{testCollection(2, 15000)}, payments)

	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-4", Amount: 10000, Comment: "Петренко Іван", OccurredAt: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := payments.created[0].MatchMethod; got != "unmatched" {
		t.Fatalf("expected unmatched, got %q", got)
	}
}

func TestPaymentMatcherIgnoresDuplicateTransaction(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{"tx-5": true}}
	matcher := newMatcher(nil, nil, payments)

	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-5", Amount: 15000, OccurredAt: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(payments.created) != 0 {
		t.Fatalf("duplicate transaction was created: %#v", payments.created)
	}
}
