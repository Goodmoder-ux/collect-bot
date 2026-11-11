package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

type sentMessage struct {
	to     string
	text   string
	markup *telebot.ReplyMarkup
}

type fakeSender struct {
	messages []sentMessage
	fail     bool
}

func (f *fakeSender) Send(to telebot.Recipient, what any, opts ...any) (*telebot.Message, error) {
	if f.fail {
		return nil, telebot.ErrBlockedByUser
	}
	message := sentMessage{to: to.Recipient(), text: what.(string)}
	for _, opt := range opts {
		if markup, ok := opt.(*telebot.ReplyMarkup); ok {
			message.markup = markup
		}
	}
	f.messages = append(f.messages, message)
	return &telebot.Message{}, nil
}

func TestPaymentMatchedNotifiesStudent(t *testing.T) {
	client := &fakeSender{}
	events := &PaymentEvents{client: client, adminID: 1}
	telegramID := int64(555)
	collection := domain.Collection{Title: "Квіти"}

	events.PaymentMatched(context.Background(), domain.Student{TelegramID: &telegramID}, collection,
		domain.Payment{Amount: 15000, MatchMethod: "auto_exact"})
	events.PaymentMatched(context.Background(), domain.Student{TelegramID: &telegramID}, collection,
		domain.Payment{Amount: 15000, MatchMethod: "manual_cash"})
	// A student without a linked Telegram account cannot be notified.
	events.PaymentMatched(context.Background(), domain.Student{}, collection, domain.Payment{Amount: 15000})

	if len(client.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(client.messages))
	}
	if got := client.messages[0]; got.to != "555" || got.text != "✅ Ваш платіж 150.00 грн зараховано до збору «Квіти». Дякуємо!" {
		t.Fatalf("auto match message = %+v", got)
	}
	if !strings.HasPrefix(client.messages[1].text, "✅ Готівковий платіж 150.00 грн") {
		t.Fatalf("cash message = %q", client.messages[1].text)
	}
}

func TestPaymentUnmatchedNotifiesAdminWithButton(t *testing.T) {
	client := &fakeSender{}
	events := &PaymentEvents{client: client, adminID: 1}
	events.PaymentUnmatched(context.Background(), domain.Payment{
		ID: 9, Amount: 20000, CommentRaw: "за квіти", ReceivedAt: time.Date(2026, 11, 11, 12, 0, 0, 0, time.Local),
	})
	if len(client.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(client.messages))
	}
	got := client.messages[0]
	if got.to != "1" {
		t.Fatalf("sent to %s, want the admin", got.to)
	}
	for _, part := range []string{"нерозпізнаний платіж", "11.11.2026 12:00", "200.00 грн", "за квіти"} {
		if !strings.Contains(got.text, part) {
			t.Fatalf("text %q lacks %q", got.text, part)
		}
	}
	button := got.markup.InlineKeyboard[0][0]
	if button.Unique != "match_payment" || button.Data != "9" {
		t.Fatalf("button = %+v, want match_payment for payment 9", button)
	}
}

func TestUnmatchedPaymentTextWithoutComment(t *testing.T) {
	if text := unmatchedPaymentText(domain.Payment{Amount: 100}); !strings.HasSuffix(text, "Коментар: без коментаря") {
		t.Fatalf("text = %q", text)
	}
}

func TestCollectionClosedNotifiesAdmin(t *testing.T) {
	client := &fakeSender{}
	(&PaymentEvents{client: client, adminID: 1}).CollectionClosed(context.Background(), domain.Collection{Title: "Квіти"})
	if len(client.messages) != 1 || client.messages[0].to != "1" || !strings.Contains(client.messages[0].text, "«Квіти» закрито") {
		t.Fatalf("messages = %+v", client.messages)
	}
}

func TestNotifyViaSurvivesBlockedUser(t *testing.T) {
	if notifyVia(&fakeSender{fail: true}, 5, "text") {
		t.Fatal("a failed delivery must be reported as false")
	}
	if !notifyVia(&fakeSender{}, 5, "text") {
		t.Fatal("a successful delivery must be reported as true")
	}
}

func TestCollectionAnnouncement(t *testing.T) {
	collection := domain.Collection{Title: "Квіти", Amount: 15000, DeadlineAt: time.Date(2026, 11, 20, 23, 59, 59, 0, time.UTC)}
	text, markup := collectionAnnouncement(collection, "https://send.monobank.ua/jar/abc")
	for _, part := range []string{"Новий збір: «Квіти»", "150.00 грн", "20.11.2026", "прізвище"} {
		if !strings.Contains(text, part) {
			t.Fatalf("text %q lacks %q", text, part)
		}
	}
	if url := markup.InlineKeyboard[0][0].URL; url != "https://send.monobank.ua/jar/abc" {
		t.Fatalf("jar button URL = %q", url)
	}
	if _, markup := collectionAnnouncement(collection, ""); len(markup.InlineKeyboard) != 0 {
		t.Fatal("no jar URL must mean no button")
	}
}

func TestCollectionCreatedText(t *testing.T) {
	tests := []struct {
		sent, total int
		want        string
	}{
		{0, 0, "ще немає"},
		{25, 25, "всі зареєстровані студенти (25)"},
		{23, 25, "23 з 25"},
	}
	for _, test := range tests {
		if text := collectionCreatedText("Квіти", test.sent, test.total); !strings.Contains(text, test.want) || !strings.Contains(text, "«Квіти» створено") {
			t.Fatalf("collectionCreatedText(%d, %d) = %q, want it to contain %q", test.sent, test.total, text, test.want)
		}
	}
}
