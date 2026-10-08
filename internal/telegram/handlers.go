package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"collect-bot/internal/domain"
	"collect-bot/internal/service"
	"gopkg.in/telebot.v3"
)

func (b *Bot) RegisterAdminStudentHandlers(
	collections domain.CollectionRepository,
	payments domain.PaymentRepository,
	reports domain.AdminReportRepository,
	reminders *service.ReminderScheduler,
	jarURL string,
) {
	collectionWizard := newCollectionWizard(collections, b.adminID)
	b.Client.Handle("/newcollection", b.RequireAdmin(collectionWizard.start))
	b.Client.Handle(telebot.OnText, collectionWizard.text)
	b.Client.Handle(&telebot.Btn{Unique: "collection_deadline"}, b.RequireAdmin(collectionWizard.deadline))
	b.Client.Handle(&telebot.Btn{Unique: "collection_confirm"}, b.RequireAdmin(collectionWizard.confirmCallback))
	b.Client.Handle(&telebot.Btn{Unique: "collection_cancel"}, b.RequireAdmin(collectionWizard.confirmCallback))
	b.Client.Handle("/collections", b.RequireStudent(func(c telebot.Context) error {
		return showCollections(c, collections, payments)
	}))
	b.Client.Handle("/status", b.RequireAdmin(func(c telebot.Context) error {
		return b.showCollectionStatus(c, collections, payments)
	}))
	b.Client.Handle("/export", b.RequireAdmin(func(c telebot.Context) error {
		return b.exportCollection(c, collections, payments)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "collection_status"}, b.RequireAdmin(func(c telebot.Context) error {
		id, err := strconv.ParseInt(c.Data(), 10, 64)
		if err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return b.sendCollectionStatus(c, id, collections, payments)
	}))
	b.Client.Handle("/payments", b.RequireStudent(func(c telebot.Context) error {
		return showCollections(c, collections, payments)
	}))
	b.Client.Handle("/jar", b.RequireStudent(func(c telebot.Context) error {
		markup := &telebot.ReplyMarkup{}
		markup.Inline(markup.Row(telebot.Btn{Text: "💳 Відкрити банку в Monobank", URL: jarURL}))
		return c.Send("Поповніть банку Monobank.\nУ коментарі обов'язково вкажіть своє українське прізвище.", markup)
	}))
	b.Client.Handle("/closecollection", b.RequireAdmin(func(c telebot.Context) error {
		return closeCollection(c, collections)
	}))
	b.Client.Handle("/unmatched", b.RequireAdmin(func(c telebot.Context) error {
		return unmatchedWorkflow{payments: payments, students: b.students, collections: collections}.list(c)
	}))
	workflow := unmatchedWorkflow{payments: payments, students: b.students, collections: collections}
	b.Client.Handle(&telebot.Btn{Unique: "match_payment"}, b.RequireAdmin(workflow.chooseStudent))
	b.Client.Handle(&telebot.Btn{Unique: "match_student"}, b.RequireAdmin(workflow.chooseCollection))
	b.Client.Handle(&telebot.Btn{Unique: "match_collection"}, b.RequireAdmin(workflow.confirm))
	b.Client.Handle(&telebot.Btn{Unique: "match_confirm"}, b.RequireAdmin(func(c telebot.Context) error {
		return workflow.execute(c, b)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "match_cancel"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Прив'язку платежу скасовано.")
	}))
	b.Client.Handle(&telebot.Btn{Unique: "status_remind"}, b.RequireAdmin(func(c telebot.Context) error {
		id, err := strconv.ParseInt(c.Data(), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid reminder collection ID: %w", err)
		}
		if err := reminders.SendDebtorReminderForCollection(context.Background(), id); err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Нагадування боржникам надіслано.")
	}))
	b.Client.Handle(&telebot.Btn{Unique: "status_close"}, b.RequireAdmin(func(c telebot.Context) error {
		id, err := strconv.ParseInt(c.Data(), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid close collection ID: %w", err)
		}
		if err := collections.CloseManually(context.Background(), id); err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Збір закрито.")
	}))
	cashWorkflow := cashPaymentWorkflow{students: b.students, collections: collections, payments: payments}
	b.Client.Handle(&telebot.Btn{Unique: "status_cash"}, b.RequireAdmin(cashWorkflow.chooseStudent))
	b.Client.Handle(&telebot.Btn{Unique: "cash_student"}, b.RequireAdmin(cashWorkflow.chooseCollection))
	b.Client.Handle(&telebot.Btn{Unique: "cash_collection"}, b.RequireAdmin(cashWorkflow.execute))
	b.Client.Handle("/match", b.RequireAdmin(func(c telebot.Context) error {
		return manuallyMatch(c, b, payments, collections)
	}))
	b.Client.Handle("/report", b.RequireAdmin(func(c telebot.Context) error {
		return sendReport(c, collections, payments, reports)
	}))
}

func createCollection(c telebot.Context, collections domain.CollectionRepository) error {
	parts := strings.SplitN(strings.TrimSpace(c.Text()), " ", 2)
	if len(parts) != 2 {
		return c.Send("Формат: /newcollection Назва|сума_в_копійках|дедлайн_RFC3339")
	}
	fields := strings.Split(parts[1], "|")
	if len(fields) != 3 {
		return c.Send("Формат: /newcollection Назва|сума_в_копійках|дедлайн_RFC3339")
	}
	amount, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
	if err != nil || amount <= 0 {
		return fmt.Errorf("invalid collection amount: %w", err)
	}
	deadline, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[2]))
	if err != nil {
		return fmt.Errorf("invalid collection deadline: %w", err)
	}
	id, err := collections.Create(context.Background(), strings.TrimSpace(fields[0]), amount, deadline)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return c.Send(fmt.Sprintf("Збір #%d створено.", id))
}

func showCollections(c telebot.Context, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
	student, ok := StudentFromContext(c)
	if !ok {
		return fmt.Errorf("student context is missing")
	}
	isAdmin := student.Role == "admin"
	active, err := collections.ListActive(context.Background())
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if len(active) == 0 {
		return c.Send("Активних зборів немає.")
	}
	var builder strings.Builder
	for _, collection := range active {
		status := "статус доступний студентам"
		if !isAdmin {
			paid, err := payments.ExistsForStudentCollection(context.Background(), student.ID, collection.ID)
			if err != nil {
				return fmt.Errorf("get payment status: %w", err)
			}
			status = "не оплачено"
			if paid {
				status = "оплачено"
			}
		}
		fmt.Fprintf(&builder, "• %s — %d коп., дедлайн %s — %s\n",
			collection.Title, collection.Amount, collection.DeadlineAt.Format("02.01.2006 15:04"), status)
		if isAdmin {
			markup := &telebot.ReplyMarkup{}
			markup.Inline(markup.Row(telebot.Btn{
				Unique: "collection_status", Text: "📊 Деталі платежів", Data: strconv.FormatInt(collection.ID, 10),
			}))
			if err := c.Send(fmt.Sprintf("• %s — %d коп., дедлайн %s — %s", collection.Title, collection.Amount,
				collection.DeadlineAt.Format("02.01.2006 15:04"), status), markup); err != nil {
				return err
			}
		}
	}
	if isAdmin {
		return nil
	}
	return c.Send(builder.String())
}

func closeCollection(c telebot.Context, collections domain.CollectionRepository) error {
	id, err := singleIDArgument(c.Text(), "/closecollection")
	if err != nil {
		return err
	}
	if err := collections.CloseManually(context.Background(), id); err != nil {
		return fmt.Errorf("close collection: %w", err)
	}
	return c.Send("Збір закрито.")
}

func listUnmatched(c telebot.Context, payments domain.PaymentRepository) error {
	items, err := payments.ListUnmatched(context.Background())
	if err != nil {
		return fmt.Errorf("list unmatched payments: %w", err)
	}
	if len(items) == 0 {
		return c.Send("Нерозпізнаних платежів немає.")
	}
	var builder strings.Builder
	for _, payment := range items {
		fmt.Fprintf(&builder, "#%d: %d коп. — %s\n", payment.ID, payment.Amount, payment.CommentRaw)
	}
	return c.Send(builder.String())
}

func manuallyMatch(c telebot.Context, b *Bot, payments domain.PaymentRepository, collections domain.CollectionRepository) error {
	fields := strings.Fields(c.Text())
	if len(fields) != 4 {
		return c.Send("Формат: /match payment_id student_id collection_id")
	}
	paymentID, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid payment ID: %w", err)
	}
	studentID, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid student ID: %w", err)
	}
	collectionID, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid collection ID: %w", err)
	}
	matcher := service.NewPaymentMatcher(b.students, collections, payments)
	if err := matcher.MatchManually(context.Background(), paymentID, studentID, collectionID); err != nil {
		return err
	}
	return c.Send("Платіж зіставлено.")
}

func sendReport(c telebot.Context, collections domain.CollectionRepository, payments domain.PaymentRepository, reports domain.AdminReportRepository) error {
	active, err := collections.ListActive(context.Background())
	if err != nil {
		return fmt.Errorf("list collections for report: %w", err)
	}
	unmatched, err := payments.ListUnmatched(context.Background())
	if err != nil {
		return fmt.Errorf("list unmatched for report: %w", err)
	}
	var builder strings.Builder
	builder.WriteString("Звіт адміністратора\n")
	fmt.Fprintf(&builder, "Активних зборів: %d\nНерозпізнаних платежів: %d", len(active), len(unmatched))
	if err := c.Send(builder.String()); err != nil {
		return fmt.Errorf("send report: %w", err)
	}
	for _, collection := range active {
		if err := reports.Create(context.Background(), collection.ID, builder.String(), time.Now()); err != nil {
			return fmt.Errorf("persist report for collection %d: %w", collection.ID, err)
		}
	}
	return nil
}

func singleIDArgument(text, command string) (int64, error) {
	fields := strings.Fields(text)
	if len(fields) != 2 || fields[0] != command {
		return 0, fmt.Errorf("format: %s ID", command)
	}
	id, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid ID %q", fields[1])
	}
	return id, nil
}
