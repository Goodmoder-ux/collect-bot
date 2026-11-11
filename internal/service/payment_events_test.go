package service

import (
	"context"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

type recordedEvents struct {
	matched   []domain.Payment
	students  []domain.Student
	unmatched []domain.Payment
	closed    []domain.Collection
}

func (r *recordedEvents) PaymentMatched(_ context.Context, student domain.Student, _ domain.Collection, payment domain.Payment) {
	r.students = append(r.students, student)
	r.matched = append(r.matched, payment)
}

func (r *recordedEvents) PaymentUnmatched(_ context.Context, payment domain.Payment) {
	r.unmatched = append(r.unmatched, payment)
}

func (r *recordedEvents) CollectionClosed(_ context.Context, collection domain.Collection) {
	r.closed = append(r.closed, collection)
}

func eventMatcher(collections *fakeCollections) (*PaymentMatcher, *recordedEvents) {
	events := &recordedEvents{}
	matcher := NewPaymentMatcher(
		&fakeStudents{students: []domain.Student{testStudent(1, "Петренко Іван")}},
		collections, &fakePayments{existing: map[string]bool{}})
	matcher.now = func() time.Time { return time.Unix(100, 0) }
	matcher.SetEvents(events)
	return matcher, events
}

func TestMatcherAnnouncesAutoMatch(t *testing.T) {
	matcher, events := eventMatcher(&fakeCollections{collections: []domain.Collection{testCollection(2, 15000)}})
	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-1", Amount: 15000, Comment: "Петренко"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.matched) != 1 || len(events.unmatched) != 0 {
		t.Fatalf("matched=%d unmatched=%d, want 1 and 0", len(events.matched), len(events.unmatched))
	}
	if events.students[0].ID != 1 || events.matched[0].ID == 0 || events.matched[0].MatchMethod != "auto_exact" {
		t.Fatalf("event = student %+v payment %+v", events.students[0], events.matched[0])
	}
	if len(events.closed) != 0 {
		t.Fatal("collection reported closed although debtors remain")
	}
}

func TestMatcherAnnouncesUnmatchedPaymentWithID(t *testing.T) {
	matcher, events := eventMatcher(&fakeCollections{collections: []domain.Collection{testCollection(2, 15000)}})
	err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-1", Amount: 99900, Comment: "хтось"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.unmatched) != 1 || len(events.matched) != 0 {
		t.Fatalf("matched=%d unmatched=%d, want 0 and 1", len(events.matched), len(events.unmatched))
	}
	if events.unmatched[0].ID == 0 {
		t.Fatal("unmatched event has no payment ID, so the admin button could not reference it")
	}
}

func TestMatcherIsQuietAboutKnownTransactions(t *testing.T) {
	events := &recordedEvents{}
	matcher := NewPaymentMatcher(&fakeStudents{}, &fakeCollections{}, &fakePayments{existing: map[string]bool{"tx-1": true}})
	matcher.SetEvents(events)
	if err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-1", Amount: 100}}); err != nil {
		t.Fatal(err)
	}
	if len(events.matched)+len(events.unmatched) != 0 {
		t.Fatal("a transaction seen before must not be announced again")
	}
}

func TestMatcherAnnouncesClosedCollection(t *testing.T) {
	matcher, events := eventMatcher(&fakeCollections{
		collections: []domain.Collection{testCollection(2, 15000)}, closeResult: true,
	})
	if err := matcher.RecordCash(context.Background(), 1, 2); err != nil {
		t.Fatal(err)
	}
	if len(events.matched) != 1 || events.matched[0].MatchMethod != "manual_cash" {
		t.Fatalf("matched = %+v, want one cash payment", events.matched)
	}
	if len(events.closed) != 1 || events.closed[0].ID != 2 {
		t.Fatalf("closed = %+v, want collection 2", events.closed)
	}
}

func TestMatcherAnnouncesManualMatch(t *testing.T) {
	matcher, events := eventMatcher(&fakeCollections{collections: []domain.Collection{testCollection(2, 15000)}})
	if err := matcher.MatchManually(context.Background(), 7, 1, 2); err != nil {
		t.Fatal(err)
	}
	if len(events.matched) != 1 {
		t.Fatalf("matched = %d, want 1", len(events.matched))
	}
	payment := events.matched[0]
	if payment.ID != 7 || payment.MatchMethod != "manual_admin" || payment.Amount != 15000 {
		t.Fatalf("payment = %+v", payment)
	}
}

func TestMatcherWorksWithoutEvents(t *testing.T) {
	matcher := NewPaymentMatcher(&fakeStudents{}, &fakeCollections{}, &fakePayments{existing: map[string]bool{}})
	matcher.SetEvents(nil)
	if err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-1", Amount: 100}}); err != nil {
		t.Fatal(err)
	}
}
