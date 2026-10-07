package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"collect-bot/internal/config"
	"collect-bot/internal/monobank"
	"collect-bot/internal/repository/sqlite"
	"collect-bot/internal/service"
	"collect-bot/internal/telegram"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(ctx, cfg.SQLiteDBPath, migrationsPath())
	if err != nil {
		return err
	}
	defer db.Close()

	students := sqlite.NewStudentRepository(db)
	collections := sqlite.NewCollectionRepository(db)
	payments := sqlite.NewPaymentRepository(db)
	reminders := sqlite.NewReminderRepository(db)
	// Repositories are concrete SQLite implementations of the domain contracts.
	reports := sqlite.NewAdminReportRepository(db)
	matcher := service.NewPaymentMatcher(students, collections, payments)
	mono := monobank.NewClient(cfg.MonobankToken)
	poller := service.NewPoller(mono, matcher, cfg.JarAccountID)

	bot, err := telegram.NewBot(cfg.TelegramToken, cfg.AdminTelegramID, students)
	if err != nil {
		return err
	}
	bot.RegisterRegistrationHandlers()
	bot.RegisterAdminStudentHandlers(collections, payments, reports, cfg.JarAccountID)
	notifier := telegram.NewNotifier(bot)
	reminder := service.NewReminderScheduler(students, collections, payments, reminders, notifier)
	deadlines := service.NewDeadlineChecker(collections, notifier, cfg.AdminTelegramID)

	errs := make(chan error, 3)
	go func() {
		if err := poller.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()
	go func() {
		if err := service.RunDailySchedulers(ctx, reminder, deadlines, nil); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()
	go bot.Start()

	select {
	case <-ctx.Done():
	case err := <-errs:
		stop()
		bot.Stop()
		return err
	}
	bot.Stop()
	return nil
}

func migrationsPath() string {
	if path := os.Getenv("MIGRATIONS_PATH"); path != "" {
		return path
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "migrations"
	}
	return filepath.Join(workingDir, "migrations")
}
