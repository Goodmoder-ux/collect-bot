package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

func (b *Bot) RegisterRegistrationHandlers() {
	b.Client.Handle("/start", b.handleStart)
	b.Client.Handle(&telebot.Btn{Unique: "register_student"}, b.handleSelection)
	b.Client.Handle(&telebot.Btn{Unique: "approve_registration"}, b.RequireAdmin(b.handleApproval(true)))
	b.Client.Handle(&telebot.Btn{Unique: "reject_registration"}, b.RequireAdmin(b.handleApproval(false)))
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

func (b *Bot) handleSelection(c telebot.Context) error {
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
	if err := b.students.CreatePendingRegistration(context.Background(), c.Sender().ID, studentID, c.Sender().Username); err != nil {
		return fmt.Errorf("registration: persist pending request: %w", err)
	}
	if err := c.Respond(); err != nil {
		return fmt.Errorf("registration: answer selection callback: %w", err)
	}
	approval := &telebot.ReplyMarkup{}
	approval.Inline(
		approval.Row(
			telebot.Btn{Unique: "approve_registration", Text: "✅ Підтвердити", Data: strconv.FormatInt(studentID, 10)},
			telebot.Btn{Unique: "reject_registration", Text: "❌ Відхилити", Data: strconv.FormatInt(studentID, 10)},
		),
		approval.Row(telebot.Btn{
			Text: "👤 Відкрити профіль",
			URL:  fmt.Sprintf("tg://user?id=%d", c.Sender().ID),
		}),
	)
	admin := &telebot.User{ID: b.adminID}
	text := fmt.Sprintf(
		"📩 Запит на реєстрацію\n\nСтудент у списку: %s\nTelegram-профіль: %s\nUsername: %s\nTelegram ID: %d",
		student.FullName,
		telegramDisplayName(c.Sender()),
		telegramUsername(c.Sender()),
		c.Sender().ID,
	)
	if _, err := b.Client.Send(admin, text, approval); err != nil {
		return fmt.Errorf("registration: notify admin: %w", err)
	}
	return c.Edit("Запит на реєстрацію надіслано адміністратору.")
}

func telegramDisplayName(user *telebot.User) string {
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name == "" {
		return "не вказано"
	}
	return name
}

func telegramUsername(user *telebot.User) string {
	if user.Username == "" {
		return "відсутній"
	}
	return "@" + user.Username
}

func (b *Bot) handleApproval(approve bool) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		studentID, err := strconv.ParseInt(strings.TrimSpace(c.Data()), 10, 64)
		if err != nil {
			return fmt.Errorf("registration: invalid approval student ID: %w", err)
		}
		request, err := b.students.GetPendingRegistrationByStudentID(context.Background(), studentID)
		if err != nil {
			return fmt.Errorf("registration: get pending request %d: %w", studentID, err)
		}
		if err := c.Respond(); err != nil {
			return fmt.Errorf("registration: answer approval callback: %w", err)
		}
		if !approve {
			if err := b.students.DeletePendingRegistration(context.Background(), studentID); err != nil {
				return fmt.Errorf("registration: delete rejected request %d: %w", studentID, err)
			}
			return c.Edit("Реєстрацію відхилено.")
		}
		if err := b.students.SetTelegramID(context.Background(), studentID, request.TelegramID); err != nil {
			return fmt.Errorf("registration: approve student %d: %w", studentID, err)
		}
		if err := b.students.DeletePendingRegistration(context.Background(), studentID); err != nil {
			return fmt.Errorf("registration: delete approved request %d: %w", studentID, err)
		}
		student := &telebot.User{ID: request.TelegramID}
		selected, err := b.students.GetByID(context.Background(), studentID)
		if err != nil {
			return fmt.Errorf("registration: get approved student %d: %w", studentID, err)
		}
		if _, err := b.Client.Send(student, fmt.Sprintf("✅ Реєстрацію підтверджено: %s", selected.FullName)); err != nil {
			return fmt.Errorf("registration: notify approved student: %w", err)
		}
		return c.Edit("Реєстрацію підтверджено.")
	}
}
