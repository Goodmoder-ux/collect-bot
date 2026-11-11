package telegram

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

const (
	genericErrorText   = "⚠️ Сталася помилка. Спробуйте ще раз трохи пізніше."
	notFoundText       = "Запис не знайдено. Можливо, його вже змінено або видалено."
	collectionDoneText = "Цей збір уже закрито."
	alreadyPaidText    = "Цей студент уже оплатив цей збір."
)

var errHandlerPanic = errors.New("telegram: handler panicked")

// userError carries a message that is safe and useful to show to the person who triggered it.
type userError struct{ text string }

func (e *userError) Error() string { return e.text }

func newUserError(format string, args ...any) error {
	return &userError{text: fmt.Sprintf(format, args...)}
}

// userMessage picks the reply for a handler error. expected is false for errors
// the user could not have caused, which deserve an error-level log entry.
func userMessage(err error) (text string, expected bool) {
	var ue *userError
	switch {
	case errors.As(err, &ue):
		return ue.text, true
	case errors.Is(err, domain.ErrCollectionNotActive):
		return collectionDoneText, true
	case errors.Is(err, domain.ErrAlreadyPaid):
		return alreadyPaidText, true
	case errors.Is(err, sql.ErrNoRows):
		return notFoundText, true
	default:
		return genericErrorText, false
	}
}

// handleError is the bot-wide error hook. Without it a failed handler is only
// logged: the user gets no reply and an inline button keeps spinning.
func handleError(err error, c telebot.Context) {
	if c == nil {
		slog.Error("telegram error", "err", err)
		return
	}
	// Stops the loading indicator; fails harmlessly if the handler already answered.
	if c.Callback() != nil {
		_ = c.Respond()
	}
	var senderID int64
	if sender := c.Sender(); sender != nil {
		senderID = sender.ID
	}
	switch {
	case errors.Is(err, ErrUnauthorized):
		// reject() has already told the user.
		slog.Info("unauthorized request", "sender", senderID)
		return
	case errors.Is(err, telebot.ErrSameMessageContent), errors.Is(err, telebot.ErrMessageNotModified):
		// A repeated tap on a button whose result is already on screen.
		return
	}
	text, expected := userMessage(err)
	if expected {
		slog.Info("handler rejected request", "err", err, "sender", senderID)
	} else {
		slog.Error("handler failed", "err", err, "sender", senderID)
	}
	if sendErr := c.Send(text); sendErr != nil {
		slog.Warn("send error message failed", "err", sendErr, "sender", senderID)
	}
}

// recoverPanics turns a handler panic into an error so one bad update cannot crash the bot.
func recoverPanics(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("handler panic", "panic", r, "stack", string(debug.Stack()))
				err = fmt.Errorf("%w: %v", errHandlerPanic, r)
			}
		}()
		return next(c)
	}
}
