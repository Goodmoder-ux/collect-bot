package telegram

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"github.com/Goodmoder-ux/collect-bot/internal/service"
	"gopkg.in/telebot.v3"
)

func (b *Bot) showCollectionStatus(c telebot.Context, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
	id, given, err := optionalIDArgument(c.Text())
	if err != nil {
		return err
	}
	if !given {
		return renderCollectionPicker(c, collections, pickStatus, false, false)
	}
	return b.sendCollectionStatus(c, id, collections, payments, false)
}

// sendCollectionStatus shows the payment status of one collection. With edit it
// replaces the message the pressed button belongs to instead of sending a new one.
func (b *Bot) sendCollectionStatus(c telebot.Context, id int64, collections domain.CollectionRepository, payments domain.PaymentRepository, edit bool) error {
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
	var collected int64
	for _, payment := range paidPayments {
		if payment.StudentID != nil {
			paid[*payment.StudentID] = payment
			collected += payment.Amount
		}
	}
	var body strings.Builder
	count := len(paid)
	total := len(students)
	percent := 0
	if total > 0 {
		percent = count * 100 / total
	}
	fmt.Fprintf(&body, "Збір: %s\nСума на людину: %s\nДедлайн: %s\n",
		collection.Title, formatMoney(collection.Amount), collection.DeadlineAt.Format("02.01.2006 15:04"))
	active := collection.Status == "active"
	if !active {
		fmt.Fprintf(&body, "Статус: %s\n", closedLabel(collection))
	}
	fmt.Fprintf(&body, "Сплачено: %d/%d (%d%%)\nЗібрано: %s\n\n", count, total, percent, formatMoney(collected))
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
	return reply(c, edit, body.String(), statusMarkup(id, active))
}

// statusMarkup builds the action buttons under a collection status. A closed
// collection can only be exported.
func statusMarkup(id int64, active bool) *telebot.ReplyMarkup {
	data := strconv.FormatInt(id, 10)
	markup := &telebot.ReplyMarkup{}
	export := telebot.Btn{Unique: "status_export", Text: "📄 Експорт CSV", Data: data}
	if !active {
		markup.Inline(
			markup.Row(export),
			markup.Row(telebot.Btn{Unique: "pick_archive", Text: "↩️ До архіву", Data: pickStatus}),
		)
		return markup
	}
	markup.Inline(
		markup.Row(
			telebot.Btn{Unique: "status_remind", Text: "🔔 Нагадати", Data: data},
			telebot.Btn{Unique: "status_cash", Text: "💵 Готівка", Data: data},
		),
		markup.Row(
			export,
			telebot.Btn{Unique: "status_close", Text: "🔒 Закрити збір", Data: data},
		),
		markup.Row(telebot.Btn{Unique: "status_back_collections", Text: "↩️ До списку зборів", Data: ""}),
	)
	return markup
}

func closedLabel(collection domain.Collection) string {
	label := "закрито"
	if collection.ClosedAt != nil {
		label += " " + collection.ClosedAt.In(time.Local).Format("02.01.2006")
	}
	if collection.ClosedReason != nil {
		switch *collection.ClosedReason {
		case "all_paid":
			label += " (усі сплатили)"
		case "manual_admin":
			label += " (вручну)"
		}
	}
	return label
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
	targetCollectionID, err := strconv.ParseInt(collectionID, 10, 64)
	if err != nil || targetCollectionID <= 0 {
		return fmt.Errorf("invalid cash collection ID %q", collectionID)
	}
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
			if collection.ID != targetCollectionID {
				continue
			}
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
	targetCollectionID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || targetCollectionID <= 0 {
		return fmt.Errorf("invalid cash collection ID %q", parts[0])
	}
	collections, err := w.collections.ListActive(context.Background())
	if err != nil {
		return err
	}
	markup := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	for _, collection := range collections {
		if collection.ID != targetCollectionID {
			continue
		}
		rows = append(rows, markup.Row(telebot.Btn{Unique: "cash_collection", Text: collection.Title, Data: parts[1] + "|" + strconv.FormatInt(collection.ID, 10)}))
	}
	if len(rows) == 0 {
		return c.Edit("Цей збір уже закритий або недоступний.")
	}
	rows = append(rows, markup.Row(telebot.Btn{Unique: "cash_back_status", Text: "↩️ Назад", Data: parts[0]}))
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
	id, given, err := optionalIDArgument(c.Text())
	if err != nil {
		return err
	}
	if !given {
		return renderCollectionPicker(c, collections, pickExport, false, false)
	}
	return b.sendCollectionExport(c, id, collections, payments)
}

func (b *Bot) sendCollectionExport(c telebot.Context, id int64, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
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
	document := &telebot.Document{File: telebot.FromReader(bytes.NewReader(buffer.Bytes())), FileName: fmt.Sprintf("collection-%d.csv", collection.ID), MIME: "text/csv",
		Caption: "Збір: " + collection.Title,
	}
	return c.Send(document)
}
