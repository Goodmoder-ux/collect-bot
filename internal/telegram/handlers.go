package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"github.com/Goodmoder-ux/collect-bot/internal/service"
	"gopkg.in/telebot.v3"
)

func (b *Bot) RegisterAdminStudentHandlers(
	collections domain.CollectionRepository,
	payments domain.PaymentRepository,
	reports domain.AdminReportRepository,
	reminders *service.ReminderScheduler,
	matcher *service.PaymentMatcher,
	jarURL string,
) {
	collectionWizard := newCollectionWizard(collections, b.adminID, func(collection domain.Collection) (int, int) {
		return b.announceCollection(collection, jarURL)
	})
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
	// Kept so the buttons on messages sent by older versions still work.
	b.Client.Handle(&telebot.Btn{Unique: "collection_status"}, b.RequireAdmin(func(c telebot.Context) error {
		id, err := parseCallbackID(c.Data(), "collection")
		if err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return b.sendCollectionStatus(c, id, collections, payments, false)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "status_back_collections"}, b.RequireAdmin(func(c telebot.Context) error {
		if err := c.Respond(); err != nil {
			return err
		}
		return renderCollectionPicker(c, collections, pickStatus, false, true)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "pick_collection"}, b.RequireAdmin(func(c telebot.Context) error {
		action, id, err := parsePickData(c.Data())
		if err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		switch action {
		case pickExport:
			return b.sendCollectionExport(c, id, collections, payments)
		case pickClose:
			return confirmCloseCollection(c, collections, id, true)
		default:
			return b.sendCollectionStatus(c, id, collections, payments, true)
		}
	}))
	b.Client.Handle(&telebot.Btn{Unique: "pick_archive"}, b.RequireAdmin(func(c telebot.Context) error {
		action, err := parsePickAction(c.Data())
		if err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return renderCollectionPicker(c, collections, action, true, true)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "pick_active"}, b.RequireAdmin(func(c telebot.Context) error {
		action, err := parsePickAction(c.Data())
		if err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return renderCollectionPicker(c, collections, action, false, true)
	}))
	b.Client.Handle(&telebot.Btn{Unique: "status_export"}, b.RequireAdmin(func(c telebot.Context) error {
		id, err := parseCallbackID(c.Data(), "collection")
		if err != nil {
			return err
		}
		if err := c.Respond(); err != nil {
			return err
		}
		return b.sendCollectionExport(c, id, collections, payments)
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
		return unmatchedWorkflow{payments: payments, students: b.students, collections: collections, matcher: matcher}.list(c)
	}))
	workflow := unmatchedWorkflow{payments: payments, students: b.students, collections: collections, matcher: matcher}
	b.Client.Handle(&telebot.Btn{Unique: "match_payment"}, b.RequireAdmin(workflow.chooseStudent))
	b.Client.Handle(&telebot.Btn{Unique: "match_student"}, b.RequireAdmin(workflow.chooseCollection))
	b.Client.Handle(&telebot.Btn{Unique: "match_collection"}, b.RequireAdmin(workflow.confirm))
	b.Client.Handle(&telebot.Btn{Unique: "match_confirm"}, b.RequireAdmin(workflow.execute))
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
	cashWorkflow := cashPaymentWorkflow{students: b.students, collections: collections, payments: payments, matcher: matcher}
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
		return b.sendCollectionStatus(c, id, collections, payments, true)
	}))
	b.Client.Handle("/report", b.RequireAdmin(func(c telebot.Context) error {
		return sendReport(c, collections, payments, reports)
	}))
}

func showCollections(c telebot.Context, collections domain.CollectionRepository, payments domain.PaymentRepository) error {
	student, ok := StudentFromContext(c)
	if !ok {
		return fmt.Errorf("student context is missing")
	}
	if student.Role == "admin" {
		return renderCollectionPicker(c, collections, pickStatus, false, false)
	}
	active, err := collections.ListActive(context.Background())
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if len(active) == 0 {
		return c.Send("Активних зборів зараз немає.\nКоли з'явиться новий збір, він буде доступний у цьому меню.")
	}
	var builder strings.Builder
	for _, collection := range active {
		paid, err := payments.ExistsForStudentCollection(context.Background(), student.ID, collection.ID)
		if err != nil {
			return fmt.Errorf("get payment status: %w", err)
		}
		status := "не оплачено"
		if paid {
			status = "оплачено"
		}
		fmt.Fprintf(&builder, "• %s — %s, дедлайн %s — %s\n",
			collection.Title, formatMoney(collection.Amount), collection.DeadlineAt.Format("02.01.2006 15:04"), status)
	}
	return c.Send(builder.String())
}

func requestCloseCollection(c telebot.Context, collections domain.CollectionRepository) error {
	id, given, err := optionalIDArgument(c.Text())
	if err != nil {
		return err
	}
	if !given {
		return renderCollectionPicker(c, collections, pickClose, false, false)
	}
	return confirmCloseCollection(c, collections, id, false)
}

func confirmCloseCollection(c telebot.Context, collections domain.CollectionRepository, id int64, edit bool) error {
	collection, err := collections.GetByID(context.Background(), id)
	if err != nil {
		return err
	}
	if collection.Status != "active" {
		return fmt.Errorf("close collection %d: %w", id, domain.ErrCollectionNotActive)
	}
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(
		telebot.Btn{Unique: "close_collection_confirm", Text: "✅ Так, закрити", Data: strconv.FormatInt(id, 10)},
		telebot.Btn{Unique: "close_collection_cancel", Text: "↩️ Скасувати", Data: strconv.FormatInt(id, 10)},
	))
	text := fmt.Sprintf("Закрити збір «%s»?\nБоржники більше не отримуватимуть нагадувань, а нові платежі не зараховуватимуться автоматично.",
		collection.Title)
	return reply(c, edit, text, markup)
}

func (b *Bot) help(c telebot.Context) error {
	student, _ := StudentFromContext(c)
	if student.Role == "admin" {
		return c.Send(
			"📚 Довідка адміністратора\n\n" +
				"/collections — активні збори, архів і деталі\n" +
				"/newcollection — створити збір через wizard\n" +
				"/status — детальний статус збору (вибір зі списку)\n" +
				"/unmatched — нерозпізнані платежі\n" +
				"/students — список, пошук та профілі студентів\n" +
				"/report — звіт адміністратора\n" +
				"/export — CSV-звіт по збору (вибір зі списку)\n" +
				"/closecollection — закрити збір із підтвердженням\n" +
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

// optionalIDArgument parses "/command" or "/command ID". given is false when no ID was typed.
func optionalIDArgument(text string) (id int64, given bool, err error) {
	fields := strings.Fields(text)
	switch len(fields) {
	case 0, 1:
		return 0, false, nil
	case 2:
		id, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || id <= 0 {
			return 0, false, newUserError("Некоректний ID: %s. Надішліть команду без ID, щоб обрати збір зі списку.", fields[1])
		}
		return id, true, nil
	default:
		return 0, false, newUserError("Забагато аргументів. Надішліть команду без ID, щоб обрати збір зі списку.")
	}
}
