package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/telebot.v3"
)

type registrationRequest struct {
	studentID  int64
	telegramID int64
	fullName   string
}

type registrationWorkflow struct {
	mu      sync.Mutex
	pending map[int64]registrationRequest
}

func (b *Bot) RegisterRegistrationHandlers() {
	workflow := &registrationWorkflow{pending: make(map[int64]registrationRequest)}
	b.Client.Handle("/start", b.handleStart)
	b.Client.Handle(&telebot.Btn{Unique: "register_student"}, workflow.handleSelection(b))
	b.Client.Handle(&telebot.Btn{Unique: "approve_registration"}, b.RequireAdmin(workflow.handleApproval(b, true)))
	b.Client.Handle(&telebot.Btn{Unique: "reject_registration"}, b.RequireAdmin(workflow.handleApproval(b, false)))
}

func (b *Bot) handleStart(c telebot.Context) error {
	if c.Sender() == nil {
		return fmt.Errorf("registration: missing sender")
	}
	if c.Sender().ID == b.adminID {
		return c.Send("Ви авторизовані як адміністратор.")
	}
	if _, err := b.students.GetByTelegramID(context.Background(), c.Sender().ID); err == nil {
		return c.Send("Ви вже зареєстровані.")
	}
	students, err := b.students.ListUnregistered(context.Background())
	if err != nil {
		return fmt.Errorf("registration: list available students: %w", err)
	}
	if len(students) == 0 {
		return c.Send("Вільних записів для реєстрації немає.")
	}
	markup := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(students))
	for _, student := range students {
		button := telebot.Btn{Unique: "register_student", Text: student.FullName, Data: strconv.FormatInt(student.ID, 10)}
		rows = append(rows, markup.Row(button))
	}
	markup.Inline(rows...)
	return c.Send("Оберіть своє ім'я зі списку:", markup)
}

func (w *registrationWorkflow) handleSelection(b *Bot) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if c.Sender() == nil {
			return fmt.Errorf("registration: missing sender")
		}
		studentID, err := strconv.ParseInt(strings.TrimSpace(c.Data()), 10, 64)
		if err != nil {
			return fmt.Errorf("registration: invalid student ID: %w", err)
		}
		student, err := b.students.GetByID(context.Background(), studentID)
		if err != nil {
			return fmt.Errorf("registration: get selected student: %w", err)
		}
		w.mu.Lock()
		w.pending[studentID] = registrationRequest{studentID: studentID, telegramID: c.Sender().ID, fullName: student.FullName}
		w.mu.Unlock()
		if err := c.Respond(); err != nil {
			return fmt.Errorf("registration: answer selection callback: %w", err)
		}
		approval := &telebot.ReplyMarkup{}
		approval.Inline(
			approval.Row(
				telebot.Btn{Unique: "approve_registration", Text: "✅ Approve", Data: strconv.FormatInt(studentID, 10)},
				telebot.Btn{Unique: "reject_registration", Text: "❌ Reject", Data: strconv.FormatInt(studentID, 10)},
			),
		)
		admin := &telebot.User{ID: b.adminID}
		text := fmt.Sprintf("📩 Запит на реєстрацію\nСтудент: %s\nTelegram ID: %d", student.FullName, c.Sender().ID)
		if _, err := b.Client.Send(admin, text, approval); err != nil {
			return fmt.Errorf("registration: notify admin: %w", err)
		}
		return c.Edit("Запит на реєстрацію надіслано адміністратору.")
	}
}

func (w *registrationWorkflow) handleApproval(b *Bot, approve bool) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		studentID, err := strconv.ParseInt(strings.TrimSpace(c.Data()), 10, 64)
		if err != nil {
			return fmt.Errorf("registration: invalid approval student ID: %w", err)
		}
		w.mu.Lock()
		request, ok := w.pending[studentID]
		if ok {
			delete(w.pending, studentID)
		}
		w.mu.Unlock()
		if !ok {
			return fmt.Errorf("registration: approval request %d not found", studentID)
		}
		if err := c.Respond(); err != nil {
			return fmt.Errorf("registration: answer approval callback: %w", err)
		}
		if !approve {
			return c.Edit("Реєстрацію відхилено.")
		}
		if err := b.students.SetTelegramID(context.Background(), request.studentID, request.telegramID); err != nil {
			return fmt.Errorf("registration: approve student %d: %w", request.studentID, err)
		}
		student := &telebot.User{ID: request.telegramID}
		if _, err := b.Client.Send(student, fmt.Sprintf("✅ Реєстрацію підтверджено: %s", request.fullName)); err != nil {
			return fmt.Errorf("registration: notify approved student: %w", err)
		}
		return c.Edit("Реєстрацію підтверджено.")
	}
}
