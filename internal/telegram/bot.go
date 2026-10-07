package telegram

import (
	"context"
	"fmt"
	"time"

	"collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

type Bot struct {
	Client   *telebot.Bot
	students domain.StudentRepository
	adminID  int64
}

func NewBot(token string, adminID int64, students domain.StudentRepository) (*Bot, error) {
	client, err := telebot.NewBot(telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}
	return &Bot{Client: client, students: students, adminID: adminID}, nil
}

func (b *Bot) Start() {
	b.Client.Start()
}

func (b *Bot) Stop() {
	b.Client.Stop()
}

type Notifier struct {
	bot *Bot
}

func NewNotifier(bot *Bot) *Notifier {
	return &Notifier{bot: bot}
}

func (n *Notifier) Send(ctx context.Context, telegramID int64, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := n.bot.Client.Send(&telebot.User{ID: telegramID}, text)
	return err
}
