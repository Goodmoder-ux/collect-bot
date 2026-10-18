package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

type PaymentMatcher struct {
	students    domain.StudentRepository
	collections domain.CollectionRepository
	payments    domain.PaymentRepository
	now         func() time.Time
}

func NewPaymentMatcher(students domain.StudentRepository, collections domain.CollectionRepository, payments domain.PaymentRepository) *PaymentMatcher {
	return &PaymentMatcher{students: students, collections: collections, payments: payments, now: time.Now}
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
	var studentID *int64
	var studentMatches int
	for _, student := range students {
		if studentMatchesComment(student.FullName, transaction.Comment) {
			studentMatches++
			id := student.ID
			studentID = &id
		}
	}

	collections, err := m.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("find collection for transaction %q: %w", transaction.ID, err)
	}
	var collectionID *int64
	var collectionMatches int
	for _, collection := range collections {
		if collection.Amount == transaction.Amount {
			collectionMatches++
			id := collection.ID
			collectionID = &id
		}
	}

	method := "unmatched"
	if studentMatches == 1 && collectionMatches == 1 {
		method = "auto_exact"
	} else {
		studentID = nil
		collectionID = nil
	}
	if err := m.payments.Create(ctx, domain.Payment{
		MonoTxID: transaction.ID, Amount: transaction.Amount, CommentRaw: transaction.Comment,
		ReceivedAt: transaction.OccurredAt, StudentID: studentID, CollectionID: collectionID,
		MatchMethod: method, MatchedAt: m.now(),
	}); err != nil {
		return fmt.Errorf("record transaction %q: %w", transaction.ID, err)
	}
	// Comments are not logged: they can contain personal names.
	slog.Info("payment recorded", "tx_id", transaction.ID, "amount", transaction.Amount, "method", method,
		"student_matches", studentMatches, "collection_matches", collectionMatches)
	if method == "auto_exact" {
		if _, err := m.collections.CloseIfAllStudentsPaid(ctx, *collectionID); err != nil {
			return fmt.Errorf("auto-close collection %d: %w", *collectionID, err)
		}
	}
	return nil
}

func (m *PaymentMatcher) MatchManually(ctx context.Context, paymentID, studentID, collectionID int64) error {
	if _, err := m.students.GetByID(ctx, studentID); err != nil {
		return fmt.Errorf("validate student %d: %w", studentID, err)
	}

	collections, err := m.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("validate collection %d: %w", collectionID, err)
	}
	validCollection := false
	for _, collection := range collections {
		if collection.ID == collectionID {
			validCollection = true
			break
		}
	}
	if !validCollection {
		return fmt.Errorf("collection %d is not active", collectionID)
	}
	if err := m.payments.UpdateMatch(ctx, paymentID, studentID, collectionID, "manual_admin", m.now()); err != nil {
		return fmt.Errorf("manually match payment %d: %w", paymentID, err)
	}
	if _, err := m.collections.CloseIfAllStudentsPaid(ctx, collectionID); err != nil {
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
		return fmt.Errorf("collection %d is not active", collectionID)
	}
	paid, err := m.payments.ExistsForStudentCollection(ctx, studentID, collectionID)
	if err != nil {
		return fmt.Errorf("check existing cash payment: %w", err)
	}
	if paid {
		return fmt.Errorf("student %d already paid collection %d", studentID, collectionID)
	}
	now := m.now()
	if err := m.payments.Create(ctx, domain.Payment{
		MonoTxID: fmt.Sprintf("cash-%d-%d-%d", collectionID, studentID, now.UnixNano()),
		Amount:   collection.Amount, CommentRaw: "Готівковий платіж",
		ReceivedAt: now, StudentID: &student.ID, CollectionID: &collection.ID,
		MatchMethod: "manual_cash", MatchedAt: now,
	}); err != nil {
		return fmt.Errorf("record cash payment: %w", err)
	}
	if _, err := m.collections.CloseIfAllStudentsPaid(ctx, collectionID); err != nil {
		return fmt.Errorf("auto-close collection after cash payment: %w", err)
	}
	return nil
}
