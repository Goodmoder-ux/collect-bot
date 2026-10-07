package telegram

import (
	"context"
	"errors"
	"fmt"

	"collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

var ErrUnauthorized = errors.New("telegram: unauthorized")

const studentContextKey = "telegram.student"

func (b *Bot) RequireAdmin(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		sender := c.Sender()
		if sender == nil || sender.ID != b.adminID {
			return b.reject(c)
		}
		c.Set(studentContextKey, domain.Student{TelegramID: &sender.ID, Role: "admin"})
		return next(c)
	}
}

func (b *Bot) RequireStudent(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		sender := c.Sender()
		if sender == nil || sender.ID == b.adminID {
			return b.reject(c)
		}
		student, err := b.students.GetByTelegramID(context.Background(), sender.ID)
		if err != nil {
			return b.reject(c)
		}
		if student.Role != "student" {
			return b.reject(c)
		}
		c.Set(studentContextKey, student)
		return next(c)
	}
}

func StudentFromContext(c telebot.Context) (domain.Student, bool) {
	value := c.Get(studentContextKey)
	student, ok := value.(domain.Student)
	return student, ok
}

func (b *Bot) reject(c telebot.Context) error {
	if c.Sender() != nil {
		_ = c.Send("Доступ заборонено.")
	}
	return fmt.Errorf("%w", ErrUnauthorized)
}
