package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"github.com/Goodmoder-ux/collect-bot/internal/service"
	"gopkg.in/telebot.v3"
)

// sender is the part of the Telegram client that notifications need.
type sender interface {
	Send(to telebot.Recipient, what any, opts ...any) (*telebot.Message, error)
}

// notify sends a best-effort message. A failure (usually a user who blocked the
// bot) is logged and reported as false; it never fails the calling action.
func (b *Bot) notify(telegramID int64, text string, opts ...any) bool {
	return notifyVia(b.Client, telegramID, text, opts...)
}

func notifyVia(client sender, telegramID int64, text string, opts ...any) bool {
	if _, err := client.Send(&telebot.User{ID: telegramID}, text, opts...); err != nil {
		slog.Warn("notification not delivered", "telegram_id", telegramID, "err", err)
		return false
	}
	return true
}

// announceCollection tells every registered student about a new collection.
// It returns how many were reached and how many are registered.
func (b *Bot) announceCollection(collection domain.Collection, jarURL string) (sent, total int) {
	students, err := b.students.ListRegistered(context.Background())
	if err != nil {
		slog.Error("announce collection: list students", "collection_id", collection.ID, "err", err)
		return 0, 0
	}
	text, markup := collectionAnnouncement(collection, jarURL)
	for _, student := range students {
		if student.TelegramID == nil {
			continue
		}
		total++
		if notifyVia(b.Client, *student.TelegramID, text, markup) {
			sent++
		}
	}
	return sent, total
}

func collectionAnnouncement(collection domain.Collection, jarURL string) (string, *telebot.ReplyMarkup) {
	text := fmt.Sprintf(
		"📢 Новий збір: «%s»\nСума: %s\nДедлайн: %s\n\n"+
			"Щоб оплатити, відкрийте банку Monobank, вкажіть суму збору і напишіть своє прізвище в коментарі до платежу.",
		collection.Title, formatMoney(collection.Amount), collection.DeadlineAt.Format("02.01.2006"))
	markup := &telebot.ReplyMarkup{}
	if jarURL != "" {
		markup.Inline(markup.Row(telebot.Btn{Text: "💳 Відкрити банку в Monobank", URL: jarURL}))
	}
	return text, markup
}

// PaymentEvents turns payment outcomes into Telegram notifications.
type PaymentEvents struct {
	client  sender
	adminID int64
}

var _ service.PaymentEvents = (*PaymentEvents)(nil)

func NewPaymentEvents(bot *Bot) *PaymentEvents {
	return &PaymentEvents{client: bot.Client, adminID: bot.adminID}
}

func (e *PaymentEvents) PaymentMatched(_ context.Context, student domain.Student, collection domain.Collection, payment domain.Payment) {
	if student.TelegramID == nil {
		return
	}
	notifyVia(e.client, *student.TelegramID, paymentMatchedText(collection, payment))
}

func paymentMatchedText(collection domain.Collection, payment domain.Payment) string {
	if payment.MatchMethod == "manual_cash" {
		return fmt.Sprintf("✅ Готівковий платіж %s зараховано до збору «%s».", formatMoney(payment.Amount), collection.Title)
	}
	return fmt.Sprintf("✅ Ваш платіж %s зараховано до збору «%s». Дякуємо!", formatMoney(payment.Amount), collection.Title)
}

func (e *PaymentEvents) PaymentUnmatched(_ context.Context, payment domain.Payment) {
	markup := &telebot.ReplyMarkup{}
	markup.Inline(markup.Row(telebot.Btn{
		Unique: "match_payment", Text: "🧩 Прив'язати", Data: strconv.FormatInt(payment.ID, 10),
	}))
	notifyVia(e.client, e.adminID, "❓ Новий нерозпізнаний платіж\n"+unmatchedPaymentText(payment), markup)
}

func (e *PaymentEvents) CollectionClosed(_ context.Context, collection domain.Collection) {
	notifyVia(e.client, e.adminID,
		fmt.Sprintf("🎉 Збір «%s» закрито автоматично: усі зареєстровані студенти сплатили.", collection.Title))
}

// unmatchedPaymentText describes a payment that waits for manual matching.
func unmatchedPaymentText(payment domain.Payment) string {
	comment := payment.CommentRaw
	if comment == "" {
		comment = "без коментаря"
	}
	return fmt.Sprintf("%s · %s\nКоментар: %s",
		payment.ReceivedAt.In(time.Local).Format("02.01.2006 15:04"), formatMoney(payment.Amount), comment)
}
