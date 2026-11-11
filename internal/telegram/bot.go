package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

type Bot struct {
	Client   *telebot.Bot
	students domain.StudentRepository
	adminID  int64
}

func NewBot(token string, adminID int64, students domain.StudentRepository) (*Bot, error) {
	client, err := telebot.NewBot(telebot.Settings{
		Token:   token,
		Poller:  &telebot.LongPoller{Timeout: 10 * time.Second},
		OnError: handleError,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}
	client.Use(recoverPanics)
	bot := &Bot{Client: client, students: students, adminID: adminID}
	if err := bot.setCommands(); err != nil {
		return nil, err
	}
	return bot, nil
}

func (b *Bot) setCommands() error {
	studentCommands := []telebot.Command{
		{Text: "collections", Description: "Активні збори та статус"},
		{Text: "jar", Description: "Реквізити банки Monobank"},
		{Text: "help", Description: "Довідка та команди"},
	}
	if err := b.Client.SetCommands(studentCommands); err != nil {
		return fmt.Errorf("telegram: set student commands: %w", err)
	}
	adminCommands := append([]telebot.Command{}, studentCommands...)
	adminCommands = append(adminCommands,
		telebot.Command{Text: "newcollection", Description: "Створити збір"},
		telebot.Command{Text: "students", Description: "Студенти та акаунти"},
		telebot.Command{Text: "cancel", Description: "Скасувати поточну дію"},
		telebot.Command{Text: "status", Description: "Деталі збору"},
		telebot.Command{Text: "export", Description: "Експорт платежів CSV"},
		telebot.Command{Text: "unmatched", Description: "Нерозпізнані платежі"},
		telebot.Command{Text: "report", Description: "Сформувати звіт"},
		telebot.Command{Text: "closecollection", Description: "Закрити збір"},
	)
	if err := b.Client.SetCommands(adminCommands, telebot.CommandScope{
		Type:   telebot.CommandScopeChat,
		ChatID: b.adminID,
	}); err != nil {
		return fmt.Errorf("telegram: set admin commands: %w", err)
	}
	return nil
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
