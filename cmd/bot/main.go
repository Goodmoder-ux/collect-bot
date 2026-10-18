package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // timezones must work in minimal container images without /usr/share/zoneinfo

	"github.com/Goodmoder-ux/collect-bot/internal/config"
	"github.com/Goodmoder-ux/collect-bot/internal/health"
	"github.com/Goodmoder-ux/collect-bot/internal/monobank"
	"github.com/Goodmoder-ux/collect-bot/internal/repository/sqlite"
	"github.com/Goodmoder-ux/collect-bot/internal/service"
	"github.com/Goodmoder-ux/collect-bot/internal/telegram"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: group-fund-bot [command]

Commands:
  run                 start the bot (default)
  backup <file.db>    write a consistent copy of the database while the bot runs
  import-students <file|->
                      add students (one full name per line) to the roster;
                      existing names are skipped, "-" reads from stdin
  healthcheck         exit 0 if the running bot reports healthy
  version             print the version
`

func main() {
	if err := dispatch(os.Args[1:]); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	command := "run"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	switch command {
	case "run":
		return run()
	case "backup":
		if len(args) != 1 {
			return errors.New("usage: group-fund-bot backup <destination.db>")
		}
		return backup(args[0])
	case "import-students":
		if len(args) != 1 {
			return errors.New("usage: group-fund-bot import-students <names.txt|->")
		}
		return importStudents(args[0])
	case "healthcheck":
		port := config.HealthPortFromEnv()
		if port == 0 {
			return nil
		}
		return health.Probe(port)
	case "version", "--version", "-version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-help", "-h":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogging(os.Stderr, cfg)
	// Deadlines typed by the admin and daily schedules use the configured timezone.
	time.Local = cfg.Location
	slog.Info("starting group-fund-bot", "version", version, "db", cfg.SQLiteDBPath, "tz", cfg.Location.String(),
		"reminder_hour", cfg.ReminderHour, "poll_interval", cfg.PollInterval)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(ctx, cfg.SQLiteDBPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close database", "err", err)
		}
	}()

	students := sqlite.NewStudentRepository(db)
	collections := sqlite.NewCollectionRepository(db)
	payments := sqlite.NewPaymentRepository(db)
	reminders := sqlite.NewReminderRepository(db)
	// Repositories are concrete SQLite implementations of the domain contracts.
	reports := sqlite.NewAdminReportRepository(db)
	pollerState := sqlite.NewPollerStateRepository(db)
	matcher := service.NewPaymentMatcher(students, collections, payments)
	mono := monobank.NewClient(cfg.MonobankToken)
	poller := service.NewPoller(mono, matcher, pollerState, cfg.JarAccountID, cfg.PollInterval)

	bot, err := telegram.NewBot(cfg.TelegramToken, cfg.AdminTelegramID, students)
	if err != nil {
		return err
	}
	bot.RegisterRegistrationHandlers()
	notifier := telegram.NewNotifier(bot)
	reminder := service.NewReminderScheduler(students, collections, payments, reminders, notifier, cfg.JarURL)
	deadlines := service.NewDeadlineChecker(collections, notifier, cfg.AdminTelegramID)
	bot.RegisterAdminStudentHandlers(collections, payments, reports, reminder, cfg.JarURL)

	errs := make(chan error, 3)
	go func() {
		if err := poller.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()
	go func() {
		if err := service.RunDailySchedulers(ctx, reminder, deadlines, cfg.Location, cfg.ReminderHour); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()
	if cfg.HealthPort > 0 {
		started := time.Now()
		checks := map[string]health.Check{
			"database": db.PingContext,
			// Allows a few missed polls plus the longest retry backoff before reporting unhealthy.
			"monobank_poll": health.Freshness(poller.LastSuccess, started, 3*cfg.PollInterval+5*time.Minute),
		}
		go func() {
			if err := health.Serve(ctx, cfg.HealthPort, health.Handler(checks)); err != nil {
				errs <- err
			}
		}()
	}
	go bot.Start()
	slog.Info("bot started")

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
	case err := <-errs:
		stop()
		bot.Stop()
		return err
	}
	bot.Stop()
	return nil
}

func backup(dest string) error {
	path := config.DBPathFromEnv()
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := sqlite.Open(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := sqlite.Backup(ctx, db, dest); err != nil {
		return err
	}
	fmt.Println("backup written to", dest)
	return nil
}

func importStudents(source string) error {
	input := os.Stdin
	if source != "-" {
		file, err := os.Open(source) //nolint:gosec // path is supplied by the operator on the command line
		if err != nil {
			return err
		}
		defer file.Close()
		input = file
	}
	var names []string
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		names = append(names, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read names: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := sqlite.Open(ctx, config.DBPathFromEnv())
	if err != nil {
		return err
	}
	defer db.Close()
	added, err := sqlite.NewStudentRepository(db).ImportNames(ctx, names)
	if err != nil {
		return err
	}
	fmt.Printf("added %d students\n", added)
	return nil
}

func setupLogging(w io.Writer, cfg config.Config) {
	options := &slog.HandlerOptions{Level: cfg.LogLevel}
	var handler slog.Handler = slog.NewTextHandler(w, options)
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(w, options)
	}
	slog.SetDefault(slog.New(handler))
}
