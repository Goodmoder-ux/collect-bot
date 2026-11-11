package telegram

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

type fakeContext struct {
	telebot.Context
	callback  *telebot.Callback
	sent      []string
	edited    []string
	markup    *telebot.ReplyMarkup
	responded int
}

func (f *fakeContext) Callback() *telebot.Callback { return f.callback }
func (f *fakeContext) Sender() *telebot.User       { return &telebot.User{ID: 42} }

func (f *fakeContext) Send(what any, opts ...any) error {
	f.sent = append(f.sent, fmt.Sprint(what))
	f.keepMarkup(opts)
	return nil
}

func (f *fakeContext) Edit(what any, opts ...any) error {
	f.edited = append(f.edited, fmt.Sprint(what))
	f.keepMarkup(opts)
	return nil
}

func (f *fakeContext) keepMarkup(opts []any) {
	for _, opt := range opts {
		if markup, ok := opt.(*telebot.ReplyMarkup); ok {
			f.markup = markup
		}
	}
}

// buttons returns "unique|data" for every inline button of the last message.
func (f *fakeContext) buttons() []string {
	var result []string
	if f.markup == nil {
		return result
	}
	for _, row := range f.markup.InlineKeyboard {
		for _, button := range row {
			result = append(result, button.Unique+"|"+button.Data)
		}
	}
	return result
}

func (f *fakeContext) Respond(_ ...*telebot.CallbackResponse) error {
	f.responded++
	return nil
}

func TestUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		want     string
		expected bool
	}{
		{"user error", fmt.Errorf("wrap: %w", newUserError("Формат: %s ID", "/status")), "Формат: /status ID", true},
		{"closed collection", fmt.Errorf("collection 3: %w", domain.ErrCollectionNotActive), collectionDoneText, true},
		{"already paid", fmt.Errorf("x: %w", domain.ErrAlreadyPaid), alreadyPaidText, true},
		{"not found", fmt.Errorf("get collection 9: %w", sql.ErrNoRows), notFoundText, true},
		{"unknown", errors.New("database is locked"), genericErrorText, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, expected := userMessage(test.err)
			if got != test.want || expected != test.expected {
				t.Fatalf("userMessage() = %q, %v; want %q, %v", got, expected, test.want, test.expected)
			}
		})
	}
}

func TestHandleErrorRepliesToCommand(t *testing.T) {
	c := &fakeContext{}
	handleError(errors.New("boom"), c)
	if len(c.sent) != 1 || c.sent[0] != genericErrorText {
		t.Fatalf("sent = %q, want the generic error text", c.sent)
	}
	if c.responded != 0 {
		t.Fatalf("responded to a callback %d times for a plain command", c.responded)
	}
}

func TestHandleErrorAnswersCallback(t *testing.T) {
	c := &fakeContext{callback: &telebot.Callback{}}
	handleError(fmt.Errorf("close: %w", domain.ErrCollectionNotActive), c)
	if c.responded != 1 {
		t.Fatalf("responded = %d, want 1 so the button stops spinning", c.responded)
	}
	if len(c.sent) != 1 || c.sent[0] != collectionDoneText {
		t.Fatalf("sent = %q, want %q", c.sent, collectionDoneText)
	}
}

func TestHandleErrorStaysQuiet(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("%w", ErrUnauthorized),
		fmt.Errorf("edit: %w", telebot.ErrSameMessageContent),
	} {
		c := &fakeContext{callback: &telebot.Callback{}}
		handleError(err, c)
		if len(c.sent) != 0 {
			t.Fatalf("handleError(%v) sent %q, want no extra message", err, c.sent)
		}
		if c.responded != 1 {
			t.Fatalf("handleError(%v) responded %d times, want 1", err, c.responded)
		}
	}
}

func TestHandleErrorWithoutContext(t *testing.T) {
	handleError(errors.New("poller failed"), nil) // must not panic
}

func TestRecoverPanics(t *testing.T) {
	handler := recoverPanics(func(telebot.Context) error { panic("nil map") })
	err := handler(&fakeContext{})
	if !errors.Is(err, errHandlerPanic) {
		t.Fatalf("err = %v, want errHandlerPanic", err)
	}
	if text, expected := userMessage(err); text != genericErrorText || expected {
		t.Fatalf("panic is reported as %q, expected=%v", text, expected)
	}
}
