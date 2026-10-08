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
	b.Client.Handle("/cancel", b.RequireAdmin(collectionWizard.cancel))
	b.Client.Handle(telebot.OnText, collectionWizard.text)
	b.Client.Handle(&telebot.Btn{Unique: "collection_deadline"}, b.RequireAdmin(collectionWizard.deadline))
	b.Client.Handle(&telebot.Btn{Unique: "collection_confirm"}, b.RequireAdmin(collectionWizard.confirmCallback))
	b.Client.Handle(&telebot.Btn{Unique: "collection_cancel"}, b.RequireAdmin(collectionWizard.confirmCallback))
	b.Client.Handle("/collections", b.RequireStudent(func(c telebot.Context) error {
		return showCollections(c, collections, payments)
	}))
	b.Client.Handle("/help", b.RequireStudent(b.help))
	b.Client.Handle("/students", b.RequireAdmin(b.listStudents))
	b.Client.Handle(&telebot.Btn{Unique: "admin_student"}, b.RequireAdmin(func(c telebot.Context) error {
		return b.showStudentProfile(c, payments, collections)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "admin_students_back"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return b.renderStudentList(c, "", true)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "admin_students_help"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Пошук: /students частина ПІБ або Telegram ID\nНаприклад: /students Петренко")
	}))
	b.Client.Handle(&telebot.Btn{Unique: "admin_student_unlink"}, b.RequireAdmin(b.confirmStudentUnlink))
	b.Client.Handle(&telebot.Btn{Unique: "admin_student_unlink_cancel"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return b.showStudentProfile(c, payments, collections)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "admin_student_unlink_confirm"}, b.RequireAdmin(func(c telebot.Context) error {
		studentID, err := parseCallbackID(c.Data(), "student")
		if err != nil {
			return err
		}
		if err := b.students.UnlinkTelegramID(context.Background(), studentID); err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("✅ Telegram-акаунт відв'язано. Історію платежів збережено.")
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
	b.Client.Handle(&telebot.Btn{Unique: "status_back_collections"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return showCollections(c, collections, payments)
	}))
	b.Client.Handle("/payments", b.RequireStudent(func(c telebot.Context) error {
		return showCollections(c, collections, payments)
	}))
	b.Client.Handle("/jar", b.RequireStudent(func(c telebot.Context) error {
		markup := &telebot.ReplyMarkup{}
		markup.Inline(markup.Row(telebot.Btn{Text: "💳 Відкрити банку в Monobank", URL: jarURL}))
		return c.Send(
			"💳 Оплата через Monobank\n\n"+
				"1. Відкрийте банку за кнопкою нижче.\n"+
				"2. Вкажіть суму збору.\n"+
				"3. У коментарі до платежу напишіть своє українське прізвище.\n\n"+
				"Після переказу платіж буде перевірено автоматично.",
			markup,
		)
	}))
	b.Client.Handle("/closecollection", b.RequireAdmin(func(c telebot.Context) error {
		return requestCloseCollection(c, collections)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "close_collection_confirm"}, b.RequireAdmin(func(c telebot.Context) error {
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
		return c.Edit("✅ Збір закрито.")
	}))
	b.Client.Handle(&telebot.Btn{Unique: "close_collection_cancel"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Закриття збору скасовано.")
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
		if err := c.Respond(); err != nil {
			return err
		}
		markup := &telebot.ReplyMarkup{}
		markup.Inline(markup.Row(
			telebot.Btn{Unique: "status_close_confirm", Text: "✅ Так, закрити", Data: strconv.FormatInt(id, 10)},
			telebot.Btn{Unique: "status_close_cancel", Text: "↩️ Назад", Data: strconv.FormatInt(id, 10)},
		))
		return c.Edit("Ви впевнені, що хочете закрити цей збір?", markup)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "status_close_confirm"}, b.RequireAdmin(func(c telebot.Context) error {
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
		return c.Edit("✅ Збір закрито.")
	}))
	b.Client.Handle(&telebot.Btn{Unique: "status_close_cancel"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return c.Edit("Закриття збору скасовано.")
	}))
	cashWorkflow := cashPaymentWorkflow{students: b.students, collections: collections, payments: payments}
	b.Client.Handle(&telebot.Btn{Unique: "status_cash"}, b.RequireAdmin(cashWorkflow.chooseStudent))
	b.Client.Handle(&telebot.Btn{Unique: "cash_student"}, b.RequireAdmin(cashWorkflow.chooseCollection))
	b.Client.Handle(&telebot.Btn{Unique: "cash_collection"}, b.RequireAdmin(cashWorkflow.execute))
	b.Client.Handle(&telebot.Btn{Unique: "cash_back_status"}, b.RequireAdmin(func(c telebot.Context) error {
		id, err := strconv.ParseInt(c.Data(), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid cash collection ID: %w", err)
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return b.sendCollectionStatus(c, id, collections, payments)
	}))
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
		return c.Send("Формат: /newcollection Назва|сума_в_гривнях|дедлайн_RFC3339")
	}
	fields := strings.Split(parts[1], "|")
	if len(fields) != 3 {
		return c.Send("Формат: /newcollection Назва|сума_в_гривнях|дедлайн_RFC3339")
	}
	amount, err := parseHryvnia(strings.TrimSpace(fields[1]))
	if err != nil {
		return fmt.Errorf("invalid collection amount in hryvnias: %w", err)
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
		return c.Send("Активних зборів зараз немає.\nКоли з'явиться новий збір, він буде доступний у цьому меню.")
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
		fmt.Fprintf(&builder, "• %s — %s, дедлайн %s — %s\n",
			collection.Title, formatMoney(collection.Amount), collection.DeadlineAt.Format("02.01.2006 15:04"), status)
		if isAdmin {
			markup := &telebot.ReplyMarkup{}
			markup.Inline(markup.Row(telebot.Btn{
				Unique: "collection_status", Text: "📊 Деталі платежів", Data: strconv.FormatInt(collection.ID, 10),
			}))
			if err := c.Send(fmt.Sprintf("• %s — %s, дедлайн %s — %s", collection.Title, formatMoney(collection.Amount),
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

func requestCloseCollection(c telebot.Context, collections domain.CollectionRepository) error {
	id, err := singleIDArgument(c.Text(), "/closecollection")
	if err != nil {
		return err
	}
	if _, err := collections.GetByID(context.Background(), id); err != nil {
		return fmt.Errorf("get collection %d: %w", id, err)
	}
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(
		telebot.Btn{Unique: "close_collection_confirm", Text: "✅ Так, закрити", Data: strconv.FormatInt(id, 10)},
		telebot.Btn{Unique: "close_collection_cancel", Text: "↩️ Скасувати", Data: strconv.FormatInt(id, 10)},
	))
	return c.Send(fmt.Sprintf("Ви впевнені, що хочете закрити збір #%d?", id), markup)
}

func (b *Bot) help(c telebot.Context) error {
	student, _ := StudentFromContext(c)
	if student.Role == "admin" {
		return c.Send(
			"📚 Довідка адміністратора\n\n" +
				"/collections — активні збори та деталі\n" +
				"/newcollection — створити збір через wizard\n" +
				"/status ID — детальний статус збору\n" +
				"/unmatched — нерозпізнані платежі\n" +
				"/students — список, пошук та профілі студентів\n" +
				"/report — звіт адміністратора\n" +
				"/export ID — CSV-звіт по збору\n" +
				"/closecollection ID — закрити збір із підтвердженням\n" +
				"/cancel — скасувати поточний wizard\n" +
				"/jar — інструкція оплати через Monobank",
		)
	}
	return c.Send(
		"📚 Довідка\n\n" +
			"/collections — активні збори та ваш статус\n" +
			"/jar — відкрити банку Monobank та інструкція оплати\n" +
			"/help — показати цю довідку\n\n" +
			"Після оплати вкажіть своє українське прізвище в коментарі до переказу.",
	)
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
	builder.WriteString("⚠️ Нерозпізнані платежі\n\n")
	for _, payment := range items {
		fmt.Fprintf(&builder, "Платіж #%d\nСума: %s\nКоментар: %s\nОтримано: %s\n\n",
			payment.ID, formatMoney(payment.Amount), payment.CommentRaw, payment.ReceivedAt.Format("02.01.2006 15:04"))
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
