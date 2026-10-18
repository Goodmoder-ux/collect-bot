package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

func (b *Bot) listStudents(c telebot.Context) error {
	query := strings.TrimSpace(strings.TrimPrefix(c.Text(), "/students"))
	return b.renderStudentList(c, query, false)
}

func (b *Bot) renderStudentList(c telebot.Context, query string, edit bool) error {
	students, err := b.students.Search(context.Background(), query)
	if err != nil {
		return fmt.Errorf("students: search: %w", err)
	}
	if len(students) == 0 {
		message := "Студентів за цим запитом не знайдено."
		if edit {
			return c.Edit(message)
		}
		return c.Send(message)
	}
	markup := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(students)+1)
	for _, student := range students {
		status := "не прив'язаний"
		if student.TelegramID != nil {
			status = "прив'язаний"
		}
		rows = append(rows, markup.Row(telebot.Btn{
			Unique: "admin_student", Text: fmt.Sprintf("%s — %s", student.FullName, status),
			Data: strconv.FormatInt(student.ID, 10),
		}))
	}
	rows = append(rows, markup.Row(telebot.Btn{Unique: "admin_students_help", Text: "ℹ️ Як шукати", Data: ""}))
	markup.Inline(rows...)
	title := "👥 Студенти\nОберіть студента для перегляду профілю."
	if query != "" {
		title = fmt.Sprintf("👥 Результати пошуку: %s", query)
	}
	if edit {
		return c.Edit(title, markup)
	}
	return c.Send(title, markup)
}

func (b *Bot) showStudentProfile(c telebot.Context, payments domain.PaymentRepository, collections domain.CollectionRepository) error {
	studentID, err := parseCallbackID(c.Data(), "student")
	if err != nil {
		return err
	}
	student, err := b.students.GetByID(context.Background(), studentID)
	if err != nil {
		return fmt.Errorf("students: get profile: %w", err)
	}
	history, err := payments.ListForStudent(context.Background(), studentID)
	if err != nil {
		return fmt.Errorf("students: list payment history: %w", err)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "👤 Профіль студента\n\nПІБ: %s\nID: %d\nTelegram: %s\n\n",
		student.FullName, student.ID, telegramIDLabel(student.TelegramID))
	body.WriteString("Історія платежів:\n")
	if len(history) == 0 {
		body.WriteString("Платежів ще немає.\n")
	} else {
		for _, payment := range history {
			collectionTitle := "збір не вказаний"
			if payment.CollectionID != nil {
				if collection, err := collections.GetByID(context.Background(), *payment.CollectionID); err == nil {
					collectionTitle = collection.Title
				}
			}
			fmt.Fprintf(&body, "• %s — %s (%s), %s\n", collectionTitle, formatMoney(payment.Amount),
				paymentMethodLabel(payment.MatchMethod), payment.MatchedAt.Format("02.01.2006 15:04"))
		}
	}
	markup := &telebot.ReplyMarkup{}
	rows := []telebot.Row{markup.Row(telebot.Btn{Unique: "admin_students_back", Text: "↩️ До списку", Data: ""})}
	if student.TelegramID != nil {
		rows = append(rows, markup.Row(telebot.Btn{
			Unique: "admin_student_unlink", Text: "🔓 Відв'язати Telegram", Data: strconv.FormatInt(student.ID, 10),
		}))
	}
	markup.Inline(rows...)
	return c.Edit(body.String(), markup)
}

func (b *Bot) confirmStudentUnlink(c telebot.Context) error {
	studentID, err := parseCallbackID(c.Data(), "student")
	if err != nil {
		return err
	}
	student, err := b.students.GetByID(context.Background(), studentID)
	if err != nil {
		return err
	}
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(
		telebot.Btn{Unique: "admin_student_unlink_confirm", Text: "✅ Так, відв'язати", Data: strconv.FormatInt(studentID, 10)},
		telebot.Btn{Unique: "admin_student_unlink_cancel", Text: "↩️ Назад", Data: strconv.FormatInt(studentID, 10)},
	))
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit(fmt.Sprintf("Відв'язати Telegram-акаунт студента %s?\nІсторія платежів збережеться.", student.FullName), markup)
}

func parseCallbackID(value, kind string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s ID %q", kind, value)
	}
	return id, nil
}

func telegramIDLabel(id *int64) string {
	if id == nil {
		return "не прив'язаний"
	}
	return strconv.FormatInt(*id, 10)
}
