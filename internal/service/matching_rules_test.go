package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

func TestSurnameForms(t *testing.T) {
	tests := []struct {
		surname string
		want    []string // forms that must be recognised besides the surname itself
	}{
		{"петренко", []string{"петренка"}},
		{"ковальський", []string{"ковальського"}},
		{"ковальська", []string{"ковальської", "ковальську"}},
		{"хмельницька", []string{"хмельницької"}},
		{"іванова", []string{"іванової"}},
		{"іванов", []string{"іванова"}},
		{"шевчук", []string{"шевчука"}},
		{"коваль", []string{"коваля"}},
		{"кравець", []string{"кравця"}},
		{"сорока", []string{"сороки"}},
		{"палій", []string{"палія"}},
		{"білий", []string{"білого"}},
	}
	for _, test := range tests {
		forms := surnameForms(test.surname)
		if forms[0] != test.surname {
			t.Fatalf("surnameForms(%q)[0] = %q, want the surname itself", test.surname, forms[0])
		}
		for _, want := range test.want {
			if !slices.Contains(forms, want) {
				t.Fatalf("surnameForms(%q) = %v, want it to contain %q", test.surname, forms, want)
			}
		}
	}
	if forms := surnameForms(""); len(forms) != 0 {
		t.Fatalf("surnameForms(\"\") = %v, want nothing", forms)
	}
}

func TestStudentMatchesCommentWithCaseForms(t *testing.T) {
	tests := []struct {
		name, comment string
		want          bool
	}{
		{"Ковальська Олена", "за Ковальської", true},
		{"Шевчук Тарас", "Від: Шевчука Т.", true},
		{"Петренко Іван", "на квіти · Від: Марія Петренко", true},
		{"Коваль Ігор", "Ковальчук", false}, // a longer surname is a different person
		{"Сорока Анна", "сорок гривень", false},
	}
	for _, test := range tests {
		if got := studentMatchesComment(test.name, test.comment); got != test.want {
			t.Fatalf("studentMatchesComment(%q, %q) = %v, want %v", test.name, test.comment, got, test.want)
		}
	}
}

func TestInflectedSurnameCollisionGoesToReview(t *testing.T) {
	// "Іванова" is both a student's surname and the genitive of "Іванов".
	payments := &fakePayments{existing: map[string]bool{}}
	matcher := newMatcher(
		[]domain.Student{testStudent(1, "Іванов Петро"), testStudent(2, "Іванова Олена")},
		[]domain.Collection{testCollection(2, 15000)}, payments)
	if err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx", Amount: 15000, Comment: "від Іванова"}}); err != nil {
		t.Fatal(err)
	}
	if payments.created[0].MatchMethod != "unmatched" {
		t.Fatalf("ambiguous surname was matched automatically: %+v", payments.created[0])
	}
}

func collectionDue(id, amount int64, deadline time.Time) domain.Collection {
	return domain.Collection{ID: id, Amount: amount, Status: "active", DeadlineAt: deadline}
}

func TestSameAmountCollectionsPickNearestUnpaidDeadline(t *testing.T) {
	day := 24 * time.Hour
	base := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	collections := []domain.Collection{
		collectionDue(5, 15000, base.Add(7*day)),
		collectionDue(6, 15000, base.Add(2*day)),
	}
	students := []domain.Student{testStudent(1, "Петренко Іван")}
	transaction := []domain.Transaction{{ID: "tx-1", Amount: 15000, Comment: "Петренко"}}

	payments := &fakePayments{existing: map[string]bool{}}
	if err := newMatcher(students, collections, payments).Process(context.Background(), transaction); err != nil {
		t.Fatal(err)
	}
	if got := payments.created[0]; got.MatchMethod != "auto_exact" || *got.CollectionID != 6 {
		t.Fatalf("first payment = %+v, want collection 6 (nearest deadline)", got)
	}

	// Once collection 6 is paid, the next payment of the same amount goes to collection 5.
	payments = &fakePayments{existing: map[string]bool{}, paid: map[[2]int64]bool{{1, 6}: true}}
	if err := newMatcher(students, collections, payments).Process(context.Background(), transaction); err != nil {
		t.Fatal(err)
	}
	if got := payments.created[0]; got.MatchMethod != "auto_exact" || *got.CollectionID != 5 {
		t.Fatalf("second payment = %+v, want collection 5", got)
	}
}

func TestRepeatedPaymentGoesToReview(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{}, paid: map[[2]int64]bool{{1, 2}: true}}
	matcher := newMatcher([]domain.Student{testStudent(1, "Петренко Іван")}, []domain.Collection{testCollection(2, 15000)}, payments)
	if err := matcher.Process(context.Background(), []domain.Transaction{{ID: "tx-again", Amount: 15000, Comment: "Петренко"}}); err != nil {
		t.Fatal(err)
	}
	got := payments.created[0]
	if got.MatchMethod != "unmatched" || got.StudentID != nil || got.CollectionID != nil {
		t.Fatalf("repeated payment = %+v, want it unmatched", got)
	}
}

func TestManualMatchRefusesMatchedPayment(t *testing.T) {
	payments := &fakePayments{
		existing: map[string]bool{},
		stored:   map[int64]domain.Payment{7: {ID: 7, MatchMethod: "auto_exact"}},
	}
	matcher := newMatcher([]domain.Student{testStudent(1, "Петренко Іван")}, []domain.Collection{testCollection(2, 15000)}, payments)
	err := matcher.MatchManually(context.Background(), 7, 1, 2)
	if !errors.Is(err, domain.ErrPaymentAlreadyMatched) {
		t.Fatalf("err = %v, want ErrPaymentAlreadyMatched", err)
	}
	if len(payments.updated) != 0 {
		t.Fatal("a matched payment was rewritten")
	}
}

func TestManualMatchRefusesStudentWhoPaid(t *testing.T) {
	payments := &fakePayments{existing: map[string]bool{}, paid: map[[2]int64]bool{{1, 2}: true}}
	matcher := newMatcher([]domain.Student{testStudent(1, "Петренко Іван")}, []domain.Collection{testCollection(2, 15000)}, payments)
	err := matcher.MatchManually(context.Background(), 7, 1, 2)
	if !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Fatalf("err = %v, want ErrAlreadyPaid", err)
	}
	if len(payments.updated) != 0 {
		t.Fatal("a second payment was assigned to a student who already paid")
	}
}
