package service

import (
	"context"
	"errors"
	"testing"
	"time"

	sqliterepository "github.com/Goodmoder-ux/collect-bot/internal/repository/sqlite"
)

func TestJarURLText(t *testing.T) {
	if got := jarURLText("https://example.com/jar"); got != "https://example.com/jar" {
		t.Fatalf("jarURLText() = %q", got)
	}
	if got := jarURLText(""); got != "Відкрийте меню та оберіть /jar." {
		t.Fatalf("jarURLText() = %q", got)
	}
}

func TestNextDailyRun(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"before hour runs today", time.Date(2026, 10, 18, 10, 0, 0, 0, kyiv), time.Date(2026, 10, 18, 15, 0, 0, 0, kyiv)},
		{"exactly at hour runs tomorrow", time.Date(2026, 10, 18, 15, 0, 0, 0, kyiv), time.Date(2026, 10, 19, 15, 0, 0, 0, kyiv)},
		{"after hour runs tomorrow", time.Date(2026, 10, 18, 16, 0, 0, 0, kyiv), time.Date(2026, 10, 19, 15, 0, 0, 0, kyiv)},
		{"end of month rolls over", time.Date(2026, 10, 31, 20, 0, 0, 0, kyiv), time.Date(2026, 11, 1, 15, 0, 0, 0, kyiv)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextDailyRun(tt.now, 15); !got.Equal(tt.want) {
				t.Fatalf("nextDailyRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

type failingNotifier struct {
	failFor map[int64]bool
	sent    []int64
}

func (n *failingNotifier) Send(_ context.Context, telegramID int64, _ string) error {
	if n.failFor[telegramID] {
		return errors.New("telegram: bot was blocked by the user")
	}
	n.sent = append(n.sent, telegramID)
	return nil
}

func TestReminderContinuesAfterFailedSend(t *testing.T) {
	fixture := newIntegrationFixture(t)
	blockedID := seedStudent(t, fixture.db, "Петренко Іван")
	activeID := seedStudent(t, fixture.db, "Шевченко Олена")
	registerStudent(t, fixture.students, blockedID, 101)
	registerStudent(t, fixture.students, activeID, 102)
	collectionID, err := fixture.collections.Create(context.Background(), "Груповий збір", 10000, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	notifier := &failingNotifier{failFor: map[int64]bool{101: true}}
	reminders := sqliterepository.NewReminderRepository(fixture.db)
	scheduler := NewReminderScheduler(fixture.students, fixture.collections, fixture.payments, reminders, notifier)

	err = scheduler.SendDebtorReminderForCollection(context.Background(), collectionID)
	if err == nil {
		t.Fatal("expected the failed send to be reported")
	}
	if len(notifier.sent) != 1 || notifier.sent[0] != 102 {
		t.Fatalf("sent to %v, want [102]", notifier.sent)
	}
	lastSent, err := reminders.GetLastSent(context.Background(), collectionID, blockedID)
	if err != nil {
		t.Fatalf("get reminder state: %v", err)
	}
	if !lastSent.IsZero() {
		t.Fatal("failed reminder must not be marked as sent")
	}
}
