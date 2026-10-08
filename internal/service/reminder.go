package service

import (
	"context"
	"fmt"
	"time"

	"collect-bot/internal/domain"
)

type Notifier interface {
	Send(ctx context.Context, telegramID int64, text string) error
}

type ReminderScheduler struct {
	students    domain.StudentRepository
	collections domain.CollectionRepository
	payments    domain.PaymentRepository
	state       domain.ReminderRepository
	notifier    Notifier
	jarURL      string
	now         func() time.Time
}

func NewReminderScheduler(
	students domain.StudentRepository,
	collections domain.CollectionRepository,
	payments domain.PaymentRepository,
	state domain.ReminderRepository,
	notifier Notifier,
	jarURL ...string,
) *ReminderScheduler {
	publicJarURL := ""
	if len(jarURL) > 0 {
		publicJarURL = jarURL[0]
	}
	return &ReminderScheduler{
		students: students, collections: collections, payments: payments,
		state: state, notifier: notifier, jarURL: publicJarURL, now: time.Now,
	}
}

func (s *ReminderScheduler) SendDebtorReminders(ctx context.Context) error {
	collections, err := s.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list collections for reminders: %w", err)
	}
	for _, collection := range collections {
		if err := s.sendForCollection(ctx, collection, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReminderScheduler) SendDebtorReminderForCollection(ctx context.Context, collectionID int64) error {
	collections, err := s.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list collection for reminder: %w", err)
	}
	for _, collection := range collections {
		if collection.ID == collectionID {
			return s.sendForCollection(ctx, collection, false)
		}
	}
	return fmt.Errorf("collection %d is not active", collectionID)
}

func (s *ReminderScheduler) sendForCollection(ctx context.Context, collection domain.Collection, enforceAge bool) error {
	now := s.now()
	if enforceAge && collection.CreatedAt.After(now.Add(-24*time.Hour)) {
		return nil
	}
	students, err := s.students.ListRegistered(ctx)
	if err != nil {
		return fmt.Errorf("list students for reminders: %w", err)
	}
	for _, student := range students {
		if student.TelegramID == nil {
			continue
		}
		paid, err := s.payments.ExistsForStudentCollection(ctx, student.ID, collection.ID)
		if err != nil {
			return fmt.Errorf("check debtor status for collection %d student %d: %w", collection.ID, student.ID, err)
		}
		if paid {
			continue
		}
		lastSent, err := s.state.GetLastSent(ctx, collection.ID, student.ID)
		if err != nil {
			return fmt.Errorf("get reminder state for collection %d student %d: %w", collection.ID, student.ID, err)
		}
		if !lastSent.IsZero() && now.Sub(lastSent) < 24*time.Hour {
			continue
		}
		text := fmt.Sprintf(
			"🔔 Нагадування: збір «%s» очікує вашу оплату.\nСума: %d.%02d грн.\n\n"+
				"💳 Для оплати відкрийте банку Monobank: /jar\n%s",
			collection.Title, collection.Amount/100, collection.Amount%100,
			jarURLText(s.jarURL),
		)
		if err := s.notifier.Send(ctx, *student.TelegramID, text); err != nil {
			return fmt.Errorf("send reminder to student %d: %w", student.ID, err)
		}

		if err := s.state.SetLastSent(ctx, collection.ID, student.ID, now); err != nil {
			return fmt.Errorf("save reminder state for collection %d student %d: %w", collection.ID, student.ID, err)
		}
	}
	return nil
}

func jarURLText(url string) string {
	if url == "" {
		return "Відкрийте меню та оберіть /jar."
	}
	return url
}

type DeadlineChecker struct {
	collections domain.CollectionRepository
	notifier    Notifier
	adminID     int64
	now         func() time.Time
}

func NewDeadlineChecker(collections domain.CollectionRepository, notifier Notifier, adminID int64) *DeadlineChecker {
	return &DeadlineChecker{collections: collections, notifier: notifier, adminID: adminID, now: time.Now}
}

func (c *DeadlineChecker) NotifyExpired(ctx context.Context) error {
	now := c.now()
	collections, err := c.collections.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list collections for deadlines: %w", err)
	}
	for _, collection := range collections {
		if collection.DeadlineAt.After(now) {
			continue
		}
		text := fmt.Sprintf("Збір «%s» досяг дедлайну (%s), але ще активний.", collection.Title, collection.DeadlineAt.Format(time.RFC3339))
		if err := c.notifier.Send(ctx, c.adminID, text); err != nil {
			return fmt.Errorf("notify admin about collection %d deadline: %w", collection.ID, err)
		}
	}
	return nil
}

func RunDailySchedulers(ctx context.Context, reminder *ReminderScheduler, deadlines *DeadlineChecker, location *time.Location) error {
	if location == nil {
		location = time.Local
	}
	for {
		now := time.Now().In(location)
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 15, 0, 0, 0, location)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			if err := reminder.SendDebtorReminders(ctx); err != nil {
				return err
			}
			if err := deadlines.NotifyExpired(ctx); err != nil {
				return err
			}
		}
	}
}
