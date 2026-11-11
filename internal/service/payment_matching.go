package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

// PaymentEvents is told about payment outcomes so people can be notified.
// Implementations handle their own failures: a notification that cannot be
// delivered must never undo or block the payment it describes.
type PaymentEvents interface {
	// PaymentMatched fires when a payment is assigned to a student and a collection.
	PaymentMatched(ctx context.Context, student domain.Student, collection domain.Collection, payment domain.Payment)
	// PaymentUnmatched fires when an incoming payment needs manual review.
	PaymentUnmatched(ctx context.Context, payment domain.Payment)
	// CollectionClosed fires when a collection closes because everyone has paid.
	CollectionClosed(ctx context.Context, collection domain.Collection)
}

type noEvents struct{}

func (noEvents) PaymentMatched(context.Context, domain.Student, domain.Collection, domain.Payment) {}
func (noEvents) PaymentUnmatched(context.Context, domain.Payment)                                  {}
func (noEvents) CollectionClosed(context.Context, domain.Collection)                               {}

type PaymentMatcher struct {
	students    domain.StudentRepository
	collections domain.CollectionRepository
	payments    domain.PaymentRepository
	events      PaymentEvents
	now         func() time.Time
}

func NewPaymentMatcher(students domain.StudentRepository, collections domain.CollectionRepository, payments domain.PaymentRepository) *PaymentMatcher {
	return &PaymentMatcher{students: students, collections: collections, payments: payments, events: noEvents{}, now: time.Now}
}

// SetEvents registers the receiver of payment notifications. Call it before the matcher is used.
func (m *PaymentMatcher) SetEvents(events PaymentEvents) {
	if events == nil {
		events = noEvents{}
	}
	m.events = events
}

// closeIfComplete closes the collection when every registered student has paid.
func (m *PaymentMatcher) closeIfComplete(ctx context.Context, collection domain.Collection) error {
	closed, err := m.collections.CloseIfAllStudentsPaid(ctx, collection.ID)
	if err != nil {
		return err
	}
	if closed {
		m.events.CollectionClosed(ctx, collection)
	}
	return nil
}

var _ PaymentProcessor = (*PaymentMatcher)(nil)

func (m *PaymentMatcher) Process(ctx context.Context, transactions []domain.Transaction) error {
	for _, transaction := range transactions {
		if err := m.processTransaction(ctx, transaction); err != nil {
			return err
		}
	}
	return nil
}

func (m *PaymentMatcher) processTransaction(ctx context.Context, transaction domain.Transaction) error {
	exists, err := m.payments.ExistsByMonoTransactionID(ctx, transaction.ID)
	if err != nil {
		return fmt.Errorf("check transaction %q idempotency: %w", transaction.ID, err)
	}
	if exists {
		return nil
	}

	students, err := m.students.ListRegistered(ctx)
	if err != nil {
		return fmt.Errorf("find student for transaction %q: %w", transaction.ID, err)
	}
	var matchedStudent domain.Student
	var studentMatches int
	for _, student := range students {
		if studentMatchesComment(student.FullName, transaction.Comment) {
			studentMatches++
			matchedStudent = student
		}
	}

	collections, err := m.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("find collection for transaction %q: %w", transaction.ID, err)
	}
	var sameAmount []domain.Collection
	for _, collection := range collections {
		if collection.Amount == transaction.Amount {
			sameAmount = append(sameAmount, collection)
		}
	}

	payment := domain.Payment{
		MonoTxID: transaction.ID, Amount: transaction.Amount, CommentRaw: transaction.Comment,
		ReceivedAt: transaction.OccurredAt, MatchMethod: "unmatched", MatchedAt: m.now(),
	}
	var matchedCollection domain.Collection
	reason := "ok"
	switch {
	case studentMatches == 0:
		reason = "no_student"
	case studentMatches > 1:
		reason = "ambiguous_student"
	case len(sameAmount) == 0:
		reason = "no_collection_with_amount"
	default:
		collection, found, err := m.unpaidCollection(ctx, matchedStudent.ID, sameAmount)
		if err != nil {
			return fmt.Errorf("find unpaid collection for transaction %q: %w", transaction.ID, err)
		}
		if !found {
			// A repeated payment: leave it for the admin instead of recording it twice.
			reason = "already_paid"
			break
		}
		matchedCollection = collection
		payment.MatchMethod = "auto_exact"
		payment.StudentID = &matchedStudent.ID
		payment.CollectionID = &matchedCollection.ID
	}
	id, err := m.payments.Create(ctx, payment)
	if err != nil {
		return fmt.Errorf("record transaction %q: %w", transaction.ID, err)
	}
	payment.ID = id
	// Comments are not logged: they can contain personal names.
	slog.Info("payment recorded", "tx_id", transaction.ID, "amount", transaction.Amount, "method", payment.MatchMethod,
		"reason", reason, "student_matches", studentMatches, "collection_matches", len(sameAmount))
	if payment.MatchMethod != "auto_exact" {
		m.events.PaymentUnmatched(ctx, payment)
		return nil
	}
	m.events.PaymentMatched(ctx, matchedStudent, matchedCollection, payment)
	if err := m.closeIfComplete(ctx, matchedCollection); err != nil {
		return fmt.Errorf("auto-close collection %d: %w", matchedCollection.ID, err)
	}
	return nil
}

// unpaidCollection picks the collection a payment should go to when several
// active ones have the same amount: the one the student has not paid yet with
// the nearest deadline. found is false when the student has paid them all.
func (m *PaymentMatcher) unpaidCollection(ctx context.Context, studentID int64, candidates []domain.Collection) (domain.Collection, bool, error) {
	var best domain.Collection
	found := false
	for _, collection := range candidates {
		paid, err := m.payments.ExistsForStudentCollection(ctx, studentID, collection.ID)
		if err != nil {
			return domain.Collection{}, false, err
		}
		if paid {
			continue
		}
		if !found || collection.DeadlineAt.Before(best.DeadlineAt) ||
			(collection.DeadlineAt.Equal(best.DeadlineAt) && collection.ID < best.ID) {
			best = collection
			found = true
		}
	}
	return best, found, nil
}

func (m *PaymentMatcher) MatchManually(ctx context.Context, paymentID, studentID, collectionID int64) error {
	student, err := m.students.GetByID(ctx, studentID)
	if err != nil {
		return fmt.Errorf("validate student %d: %w", studentID, err)
	}
	payment, err := m.payments.GetByID(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("validate payment %d: %w", paymentID, err)
	}

	collections, err := m.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("validate collection %d: %w", collectionID, err)
	}
	var collection domain.Collection
	validCollection := false
	for _, item := range collections {
		if item.ID == collectionID {
			collection = item
			validCollection = true
			break
		}
	}
	if !validCollection {
		return fmt.Errorf("collection %d: %w", collectionID, domain.ErrCollectionNotActive)
	}
	if payment.MatchMethod != "unmatched" {
		return fmt.Errorf("payment %d: %w", paymentID, domain.ErrPaymentAlreadyMatched)
	}
	paid, err := m.payments.ExistsForStudentCollection(ctx, studentID, collectionID)
	if err != nil {
		return fmt.Errorf("check existing payment before manual match: %w", err)
	}
	if paid {
		return fmt.Errorf("student %d, collection %d: %w", studentID, collectionID, domain.ErrAlreadyPaid)
	}
	now := m.now()
	if err := m.payments.UpdateMatch(ctx, paymentID, studentID, collectionID, "manual_admin", now); err != nil {
		return fmt.Errorf("manually match payment %d: %w", paymentID, err)
	}
	payment.StudentID, payment.CollectionID = &student.ID, &collection.ID
	payment.MatchMethod, payment.MatchedAt = "manual_admin", now
	m.events.PaymentMatched(ctx, student, collection, payment)
	if err := m.closeIfComplete(ctx, collection); err != nil {
		return fmt.Errorf("auto-close collection %d after manual match: %w", collectionID, err)
	}
	return nil
}

func (m *PaymentMatcher) RecordCash(ctx context.Context, studentID, collectionID int64) error {
	student, err := m.students.GetByID(ctx, studentID)
	if err != nil {
		return fmt.Errorf("validate cash payment student %d: %w", studentID, err)
	}
	collections, err := m.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("validate cash payment collection %d: %w", collectionID, err)
	}
	var collection domain.Collection
	found := false
	for _, item := range collections {
		if item.ID == collectionID {
			collection = item
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("collection %d: %w", collectionID, domain.ErrCollectionNotActive)
	}
	paid, err := m.payments.ExistsForStudentCollection(ctx, studentID, collectionID)
	if err != nil {
		return fmt.Errorf("check existing cash payment: %w", err)
	}
	if paid {
		return fmt.Errorf("student %d, collection %d: %w", studentID, collectionID, domain.ErrAlreadyPaid)
	}
	now := m.now()
	payment := domain.Payment{
		MonoTxID: fmt.Sprintf("cash-%d-%d-%d", collectionID, studentID, now.UnixNano()),
		Amount:   collection.Amount, CommentRaw: "Готівковий платіж",
		ReceivedAt: now, StudentID: &student.ID, CollectionID: &collection.ID,
		MatchMethod: "manual_cash", MatchedAt: now,
	}
	id, err := m.payments.Create(ctx, payment)
	if err != nil {
		return fmt.Errorf("record cash payment: %w", err)
	}
	payment.ID = id
	m.events.PaymentMatched(ctx, student, collection, payment)
	if err := m.closeIfComplete(ctx, collection); err != nil {
		return fmt.Errorf("auto-close collection after cash payment: %w", err)
	}
	return nil
}
