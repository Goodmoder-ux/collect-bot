package telegram

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"collect-bot/internal/domain"
	"collect-bot/internal/service"
	"gopkg.in/telebot.v3"
)

func (b *Bot) showCollectionStatus(c telebot.Context, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
	id, err := singleIDArgument(c.Text(), "/status")
	if err != nil {
		return err
	}
	return b.sendCollectionStatus(c, id, collections, payments)
}

func (b *Bot) sendCollectionStatus(c telebot.Context, id int64, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
	collection, err := collections.GetByID(context.Background(), id)
	if err != nil {
		return err
	}
	students, err := b.students.ListRegistered(context.Background())
	if err != nil {
		return err
	}
	paidPayments, err := payments.ListForCollection(context.Background(), id)
	if err != nil {
		return err
	}
	paid := make(map[int64]domain.Payment)
	for _, payment := range paidPayments {
		if payment.StudentID != nil {
			paid[*payment.StudentID] = payment
		}
	}
	var body strings.Builder
	count := len(paid)
	total := len(students)
	percent := 0
	if total > 0 {
		percent = count * 100 / total
	}
	fmt.Fprintf(&body, "Збір: %s\nСума на людину: %s\nДедлайн: %s\nСплачено: %d/%d (%d%%)\n\n",
		collection.Title, formatMoney(collection.Amount), collection.DeadlineAt.Format("02.01.2006 15:04"), count, total, percent)
	body.WriteString("Сплатили:\n")
	for _, student := range students {
		payment, ok := paid[student.ID]
		if !ok {
			continue
		}
		fmt.Fprintf(&body, "✅ %s — %s\n", student.FullName, paymentMethodLabel(payment.MatchMethod))
	}
	body.WriteString("\nБоржники:\n")
	for _, student := range students {
		if _, ok := paid[student.ID]; !ok {
			fmt.Fprintf(&body, "❌ %s\n", student.FullName)
		}
	}
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(
		telebot.Btn{Unique: "status_remind", Text: "🔔 Нагадати боржникам", Data: strconv.FormatInt(id, 10)},
		telebot.Btn{Unique: "status_cash", Text: "💵 Додати готівковий платіж", Data: strconv.FormatInt(id, 10)},
		telebot.Btn{Unique: "status_close", Text: "🔒 Закрити збір", Data: strconv.FormatInt(id, 10)},
	))
	return c.Send(body.String(), markup)
}

func paymentMethodLabel(method string) string {
	switch method {
	case "auto_exact":
		return "Авто"
	case "manual_cash":
		return "Готівка"
	case "manual_admin":
		return "Вручну"
	default:
		return method
	}
}

type cashPaymentWorkflow struct {
	students    domain.StudentRepository
	collections domain.CollectionRepository
	payments    domain.PaymentRepository
}

func (w cashPaymentWorkflow) chooseStudent(c telebot.Context) error {
	collectionID := c.Data()
	students, err := w.students.ListRegistered(context.Background())
	if err != nil {
		return err
	}
	active, err := w.collections.ListActive(context.Background())
	if err != nil {
		return err
	}
	markup := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	for _, student := range students {
		unpaid := false
		for _, collection := range active {
			paid, err := w.payments.ExistsForStudentCollection(context.Background(), student.ID, collection.ID)
			if err != nil {
				return err
			}
			if !paid {
				unpaid = true
				break
			}
		}
		if unpaid {
			rows = append(rows, markup.Row(telebot.Btn{Unique: "cash_student", Text: student.FullName, Data: collectionID + "|" + strconv.FormatInt(student.ID, 10)}))
		}
	}
	if len(rows) == 0 {
		return c.Edit("Немає непогашених студентів.")
	}
	markup.Inline(rows...)
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit("Оберіть студента для готівкового платежу:", markup)
}

func (w cashPaymentWorkflow) chooseCollection(c telebot.Context) error {
	parts := strings.Split(c.Data(), "|")
	if len(parts) != 2 {
		return fmt.Errorf("invalid cash student selection")
	}
	collections, err := w.collections.ListActive(context.Background())
	if err != nil {
		return err
	}
	markup := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	for _, collection := range collections {
		rows = append(rows, markup.Row(telebot.Btn{Unique: "cash_collection", Text: collection.Title, Data: parts[1] + "|" + strconv.FormatInt(collection.ID, 10)}))
	}
	markup.Inline(rows...)
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit("Оберіть збір:", markup)
}

func (w cashPaymentWorkflow) execute(c telebot.Context) error {
	parts := strings.Split(c.Data(), "|")
	if len(parts) != 2 {
		return fmt.Errorf("invalid cash collection selection")
	}
	studentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return err
	}
	collectionID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return err
	}
	matcher := service.NewPaymentMatcher(w.students, w.collections, w.payments)
	if err := matcher.RecordCash(context.Background(), studentID, collectionID); err != nil {
		return err
	}
	if err := c.Respond(); err != nil {
		return err
	}
	return c.Edit("Готівковий платіж додано.")
}

func (b *Bot) exportCollection(c telebot.Context, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
	id, err := singleIDArgument(c.Text(), "/export")
	if err != nil {
		return err
	}
	collection, err := collections.GetByID(context.Background(), id)
	if err != nil {
		return err
	}
	students, err := b.students.ListRegistered(context.Background())
	if err != nil {
		return err
	}
	items, err := payments.ListForCollection(context.Background(), id)
	if err != nil {
		return err
	}
	byStudent := make(map[int64]domain.Payment)
	for _, payment := range items {
		if payment.StudentID != nil {
			byStudent[*payment.StudentID] = payment
		}
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"Student Full Name", "Payment Status (Paid/Unpaid)", "Amount (UAH)", "Payment Method", "Paid At Timestamp"}); err != nil {
		return err
	}
	for _, student := range students {
		payment, ok := byStudent[student.ID]
		row := []string{student.FullName, "Unpaid", formatMoneyValue(collection.Amount), "", ""}
		if ok {
			row[1] = "Paid"
			row[3] = paymentMethodLabel(payment.MatchMethod)
			row[4] = payment.MatchedAt.Format(time.RFC3339)
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	document := &telebot.Document{File: telebot.FromReader(bytes.NewReader(buffer.Bytes())), FileName: fmt.Sprintf("collection-%d.csv", collection.ID), MIME: "text/csv"}
	return c.Send(document)
}
