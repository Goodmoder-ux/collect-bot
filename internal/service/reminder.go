package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

// DefaultReminderHour is the local hour when daily reminders and deadline checks run.
const DefaultReminderHour = 15

// reminderCooldown is shorter than a day so timer jitter cannot skip a daily run.
const reminderCooldown = 20 * time.Hour

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
	var errs []error
	for _, collection := range collections {
		if err := s.sendForCollection(ctx, collection, true); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
	// A failed send to one student (e.g. they blocked the bot) must not stop the others.
	var sendErrs []error
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
		if !lastSent.IsZero() && now.Sub(lastSent) < reminderCooldown {
			continue
		}
		text := fmt.Sprintf(
			"🔔 Нагадування: збір «%s» очікує вашу оплату.\nСума: %d.%02d грн.\n\n"+
				"💳 Для оплати відкрийте банку Monobank: /jar\n%s",
			collection.Title, collection.Amount/100, collection.Amount%100,
			jarURLText(s.jarURL),
		)
		if err := s.notifier.Send(ctx, *student.TelegramID, text); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("send reminder failed", "collection_id", collection.ID, "student_id", student.ID, "err", err)
			sendErrs = append(sendErrs, fmt.Errorf("send reminder to student %d: %w", student.ID, err))
			continue
		}

		if err := s.state.SetLastSent(ctx, collection.ID, student.ID, now); err != nil {
			return fmt.Errorf("save reminder state for collection %d student %d: %w", collection.ID, student.ID, err)
		}
	}
	return errors.Join(sendErrs...)
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

// RunDailySchedulers runs reminders and deadline checks every day at hour in location
// until ctx is cancelled. Job failures are logged and retried on the next run.
func RunDailySchedulers(ctx context.Context, reminder *ReminderScheduler, deadlines *DeadlineChecker, location *time.Location, hour int) error {
	if location == nil {
		location = time.Local
	}
	for {
		next := nextDailyRun(time.Now().In(location), hour)
		slog.Info("next daily run scheduled", "at", next)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			if err := reminder.SendDebtorReminders(ctx); err != nil {
				slog.Error("daily debtor reminders failed", "err", err)
			}
			if err := deadlines.NotifyExpired(ctx); err != nil {
				slog.Error("daily deadline check failed", "err", err)
			}
		}
	}
}

// nextDailyRun returns the next time after now at hour:00 in now's location.
func nextDailyRun(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = time.Date(now.Year(), now.Month(), now.Day()+1, hour, 0, 0, 0, now.Location())
	}
	return next
}
