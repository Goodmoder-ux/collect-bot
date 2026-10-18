package telegram

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"github.com/Goodmoder-ux/collect-bot/internal/service"
	"gopkg.in/telebot.v3"
)

type collectionWizardStep uint8

const (
	collectionWizardTitle collectionWizardStep = iota + 1
	collectionWizardAmount
	collectionWizardDate
)

type collectionWizardState struct {
	step     collectionWizardStep
	title    string
	amount   int64
	deadline time.Time
}

type collectionWizard struct {
	mu    sync.Mutex
	state map[int64]collectionWizardState
	repo  domain.CollectionRepository
	admin int64
}

func newCollectionWizard(repo domain.CollectionRepository, adminID int64) *collectionWizard {
	return &collectionWizard{state: make(map[int64]collectionWizardState), repo: repo, admin: adminID}
}

func (w *collectionWizard) start(c telebot.Context) error {
	if c.Sender() == nil {
		return fmt.Errorf("collection wizard: missing sender")
	}
	w.mu.Lock()
	w.state[c.Sender().ID] = collectionWizardState{step: collectionWizardTitle}
	w.mu.Unlock()
	return c.Send("Крок 1/3. Введіть назву збору:")
}

func (w *collectionWizard) cancel(c telebot.Context) error {
	if c.Sender() == nil {
		return fmt.Errorf("collection wizard: missing sender")
	}
	w.mu.Lock()
	_, ok := w.state[c.Sender().ID]
	delete(w.state, c.Sender().ID)
	w.mu.Unlock()
	if !ok {
		return c.Send("Немає активної операції для скасування.")
	}
	return c.Send("Створення збору скасовано.")
}

func (w *collectionWizard) text(c telebot.Context) error {
	if c.Sender() == nil {
		return nil
	}
	if c.Sender().ID != w.admin {
		return nil
	}
	w.mu.Lock()
	state, ok := w.state[c.Sender().ID]
	w.mu.Unlock()
	if !ok {
		return nil
	}
	value := strings.TrimSpace(c.Text())
	if value == "" {
		return c.Send("Значення не може бути порожнім. Спробуйте ще раз або скасуйте операцію командою /cancel:")
	}
	switch state.step {
	case collectionWizardTitle:
		state.title = value
		state.step = collectionWizardAmount
		w.save(c.Sender().ID, state)
		return c.Send("Крок 2/3. Введіть суму в гривнях (наприклад, 100 або 100.50):")
	case collectionWizardAmount:
		amount, err := parseHryvnia(value)
		if err != nil {
			return c.Send("Некоректна сума. Введіть додатне число гривень, наприклад 100 або 100.50:")
		}
		state.amount = amount
		state.step = collectionWizardDate
		w.save(c.Sender().ID, state)
		return c.Send("Крок 3/3. Оберіть дедлайн:", collectionDeadlineMarkup())
	case collectionWizardDate:
		deadline, err := time.ParseInLocation("2006-01-02", value, time.Local)
		if err != nil {
			return c.Send("Некоректна дата. Введіть дату у форматі YYYY-MM-DD:")
		}
		state.deadline = endOfDay(deadline)
		w.save(c.Sender().ID, state)
		return w.confirm(c, state)
	default:
		return nil
	}
}

func (w *collectionWizard) deadline(c telebot.Context) error {
	if c.Sender() == nil {
		return fmt.Errorf("collection wizard: missing sender")
	}
	w.mu.Lock()
	state, ok := w.state[c.Sender().ID]
	w.mu.Unlock()
	if !ok || state.step != collectionWizardDate {
		return c.Respond()
	}
	switch c.Data() {
	case "3d":
		state.deadline = endOfDay(time.Now().AddDate(0, 0, 3))
	case "7d":
		state.deadline = endOfDay(time.Now().AddDate(0, 0, 7))
	case "custom":
		w.save(c.Sender().ID, state)
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Send("Введіть власну дату у форматі YYYY-MM-DD.\nНаприклад: 2026-12-31")
	default:
		return fmt.Errorf("collection wizard: unknown deadline option %q", c.Data())
	}
	w.save(c.Sender().ID, state)
	if err := c.Respond(); err != nil {
		return fmt.Errorf("collection wizard: answer deadline callback: %w", err)
	}
	return w.confirm(c, state)
}

func (w *collectionWizard) confirm(c telebot.Context, state collectionWizardState) error {
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(
		telebot.Btn{Unique: "collection_confirm", Text: "✅ Створити збір", Data: "confirm"},
		telebot.Btn{Unique: "collection_cancel", Text: "❌ Скасувати", Data: "cancel"},
	))
	return c.Send(fmt.Sprintf(
		"Перевірте дані збору:\nНазва: %s\nСума: %s\nДедлайн: %s",
		state.title, formatMoney(state.amount), state.deadline.Format("02.01.2006 15:04")), markup)
}

func (w *collectionWizard) confirmCallback(c telebot.Context) error {
	if c.Sender() == nil {
		return fmt.Errorf("collection wizard: missing sender")
	}
	w.mu.Lock()
	state, ok := w.state[c.Sender().ID]
	if ok {
		delete(w.state, c.Sender().ID)
	}
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("collection wizard: no active collection")
	}
	if err := c.Respond(); err != nil {
		return err
	}
	if c.Data() == "cancel" {
		return c.Edit("Створення збору скасовано.")
	}
	id, err := w.repo.Create(context.Background(), state.title, state.amount, state.deadline)
	if err != nil {
		return fmt.Errorf("collection wizard: create collection: %w", err)
	}
	return c.Edit(fmt.Sprintf("Збір #%d створено.", id))
}

func (w *collectionWizard) save(telegramID int64, state collectionWizardState) {
	w.mu.Lock()
	w.state[telegramID] = state
	w.mu.Unlock()
}

func collectionDeadlineMarkup() *telebot.ReplyMarkup {
	markup := &telebot.ReplyMarkup{}
	markup.Inline(
		markup.Row(telebot.Btn{Unique: "collection_deadline", Text: "Через 3 дні", Data: "3d"}),
		markup.Row(telebot.Btn{Unique: "collection_deadline", Text: "Через тиждень", Data: "7d"}),
		markup.Row(telebot.Btn{Unique: "collection_deadline", Text: "Своя дата (YYYY-MM-DD)", Data: "custom"}),
	)
	return markup
}

func parseHryvnia(value string) (int64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid amount")
	}
	if strings.HasPrefix(parts[0], "-") {
		return 0, fmt.Errorf("amount must be positive")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("invalid whole amount")
	}
	fraction := "0"
	if len(parts) == 2 {
		if len(parts[1]) > 2 || parts[1] == "" {
			return 0, fmt.Errorf("invalid kopecks")
		}
		fraction = parts[1]
	}
	if len(fraction) == 1 {
		fraction += "0"
	}
	if len(fraction) == 0 {
		fraction = "00"
	}
	kopecks, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil || kopecks < 0 {
		return 0, fmt.Errorf("invalid kopecks")
	}
	if whole > (math.MaxInt64-kopecks)/100 {
		return 0, fmt.Errorf("amount overflows integer")
	}
	amount := whole*100 + kopecks
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	return amount, nil
}

func endOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 23, 59, 59, 0, value.Location())
}

type unmatchedWorkflow struct {
	payments    domain.PaymentRepository
	students    domain.StudentRepository
	collections domain.CollectionRepository
}

func (w unmatchedWorkflow) list(c telebot.Context) error {
	items, err := w.payments.ListUnmatched(context.Background())
	if err != nil {
		return fmt.Errorf("unmatched workflow: list payments: %w", err)
	}
	if len(items) == 0 {
		return c.Send("Нерозпізнаних платежів немає.")
	}
	for _, payment := range items {
		markup := &telebot.ReplyMarkup{}
		markup.Inline(markup.Row(telebot.Btn{
			Unique: "match_payment", Text: "🧩 Прив'язати", Data: strconv.FormatInt(payment.ID, 10),
		}))
		if _, err := c.Bot().Send(c.Recipient(), fmt.Sprintf("#%d: %s — %s", payment.ID, formatMoney(payment.Amount), payment.CommentRaw), markup); err != nil {
			return fmt.Errorf("unmatched workflow: send payment %d: %w", payment.ID, err)
		}
	}
	return nil
}

func (w unmatchedWorkflow) chooseStudent(c telebot.Context) error {
	paymentID, err := strconv.ParseInt(c.Data(), 10, 64)
	if err != nil {
		return fmt.Errorf("unmatched workflow: invalid payment ID: %w", err)
	}
	students, err := w.students.ListRegistered(context.Background())
	if err != nil {
		return fmt.Errorf("unmatched workflow: list students: %w", err)
	}
	active, err := w.collections.ListActive(context.Background())
	if err != nil {
		return fmt.Errorf("unmatched workflow: list active collections: %w", err)
	}
	markup := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(students))
	for _, student := range students {
		unpaid := false
		for _, collection := range active {
			paid, err := w.payments.ExistsForStudentCollection(context.Background(), student.ID, collection.ID)
			if err != nil {
				return fmt.Errorf("unmatched workflow: check student %d payment: %w", student.ID, err)
			}
			if !paid {
				unpaid = true
				break
			}
		}
		if !unpaid {
			continue
		}
		rows = append(rows, markup.Row(telebot.Btn{
			Unique: "match_student", Text: student.FullName,
			Data: fmt.Sprintf("%d|%d", paymentID, student.ID),
		}))
	}
	if len(rows) == 0 {
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Немає непогашених студентів для вибору.")
	}
	markup.Inline(rows...)
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit("Оберіть студента для платежу:", markup)
}

func (w unmatchedWorkflow) chooseCollection(c telebot.Context) error {
	parts := strings.Split(c.Data(), "|")
	if len(parts) != 2 {
		return fmt.Errorf("unmatched workflow: invalid student selection")
	}
	collections, err := w.collections.ListActive(context.Background())
	if err != nil {
		return fmt.Errorf("unmatched workflow: list collections: %w", err)
	}
	markup := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(collections))
	for _, collection := range collections {
		rows = append(rows, markup.Row(telebot.Btn{
			Unique: "match_collection", Text: fmt.Sprintf("%s (%s)", collection.Title, formatMoney(collection.Amount)),
			Data: fmt.Sprintf("%s|%d", c.Data(), collection.ID),
		}))
	}
	markup.Inline(rows...)
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit("Оберіть активний збір:", markup)
}

func (w unmatchedWorkflow) confirm(c telebot.Context) error {
	parts := strings.Split(c.Data(), "|")
	if len(parts) != 3 {
		return fmt.Errorf("unmatched workflow: invalid collection selection")
	}
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(
		telebot.Btn{Unique: "match_confirm", Text: "✅ Підтвердити", Data: c.Data()},
		telebot.Btn{Unique: "match_cancel", Text: "❌ Скасувати", Data: c.Data()},
	))
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit(fmt.Sprintf("Підтвердити прив'язку платежу #%s до студента #%s та збору #%s?", parts[0], parts[1], parts[2]), markup)
}

func (w unmatchedWorkflow) execute(c telebot.Context, b *Bot) error {
	parts := strings.Split(c.Data(), "|")
	if len(parts) != 3 {
		return fmt.Errorf("unmatched workflow: invalid confirmation")
	}
	paymentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return fmt.Errorf("unmatched workflow: invalid payment ID: %w", err)
	}
	studentID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("unmatched workflow: invalid student ID: %w", err)
	}
	collectionID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return fmt.Errorf("unmatched workflow: invalid collection ID: %w", err)
	}
	matcher := service.NewPaymentMatcher(w.students, w.collections, w.payments)
	if err := matcher.MatchManually(context.Background(), paymentID, studentID, collectionID); err != nil {
		return fmt.Errorf("unmatched workflow: match payment: %w", err)
	}
	student, err := w.students.GetByID(context.Background(), studentID)
	if err != nil {
		return fmt.Errorf("unmatched workflow: get student: %w", err)
	}
	if student.TelegramID != nil {
		if _, err := b.Client.Send(&telebot.User{ID: *student.TelegramID}, "✅ Ваш платіж підтверджено та прив'язано до збору."); err != nil {
			return fmt.Errorf("unmatched workflow: notify student: %w", err)
		}
	}
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit("Платіж успішно прив'язано.")
}
