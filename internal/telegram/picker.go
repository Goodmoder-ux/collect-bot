package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
	"gopkg.in/telebot.v3"
)

// Actions a collection can be picked for. They travel in callback data.
const (
	pickStatus = "status"
	pickExport = "export"
	pickClose  = "close"
)

// archiveLimit caps the archive list so it fits in one inline keyboard.
const archiveLimit = 20

// renderCollectionPicker lists collections as buttons so the admin never has to
// type a collection ID. archive switches from active to closed collections.
func renderCollectionPicker(c telebot.Context, collections domain.CollectionRepository, action string, archive, edit bool) error {
	var (
		items []domain.Collection
		err   error
	)
	if archive {
		items, err = collections.ListClosed(context.Background(), archiveLimit)
	} else {
		items, err = collections.ListActive(context.Background())
	}
	if err != nil {
		return fmt.Errorf("collection picker: %w", err)
	}
	markup := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0, len(items)+1)
	for _, collection := range items {
		rows = append(rows, markup.Row(telebot.Btn{
			Unique: "pick_collection", Text: pickerLabel(collection),
			Data: action + "|" + strconv.FormatInt(collection.ID, 10),
		}))
	}
	switch {
	case archive:
		rows = append(rows, markup.Row(telebot.Btn{Unique: "pick_active", Text: "↩️ Активні збори", Data: action}))
	case action != pickClose:
		// Closed collections cannot be closed again, so that picker has no archive.
		rows = append(rows, markup.Row(telebot.Btn{Unique: "pick_archive", Text: "🗄 Архів", Data: action}))
	}
	markup.Inline(rows...)
	return reply(c, edit, pickerTitle(action, archive, len(items)), markup)
}

func pickerTitle(action string, archive bool, count int) string {
	var title string
	switch action {
	case pickExport:
		title = "📄 Експорт CSV"
	case pickClose:
		title = "🔒 Закриття збору"
	default:
		title = "📋 Збори"
	}
	switch {
	case archive && count == 0:
		return title + " — архів\n\nЗакритих зборів ще немає."
	case archive:
		return fmt.Sprintf("%s — архів\n\nОберіть закритий збір (показано останні %d).", title, archiveLimit)
	case count == 0:
		return title + "\n\nАктивних зборів зараз немає. Створити збір: /newcollection"
	default:
		return title + "\n\nОберіть активний збір."
	}
}

func pickerLabel(collection domain.Collection) string {
	if collection.Status == "active" {
		return fmt.Sprintf("%s — %s, до %s", collection.Title, formatMoney(collection.Amount),
			collection.DeadlineAt.Format("02.01"))
	}
	// Kept short: Telegram truncates long button captions.
	label := collection.Title + " — закрито"
	if collection.ClosedAt != nil {
		label += " " + collection.ClosedAt.In(time.Local).Format("02.01.2006")
	}
	return label
}

func parsePickAction(value string) (string, error) {
	switch value {
	case pickStatus, pickExport, pickClose:
		return value, nil
	default:
		return "", fmt.Errorf("collection picker: unknown action %q", value)
	}
}

func parsePickData(data string) (string, int64, error) {
	rawAction, rawID, ok := strings.Cut(data, "|")
	if !ok {
		return "", 0, fmt.Errorf("collection picker: invalid data %q", data)
	}
	action, err := parsePickAction(rawAction)
	if err != nil {
		return "", 0, err
	}
	id, err := parseCallbackID(rawID, "collection")
	if err != nil {
		return "", 0, err
	}
	return action, id, nil
}

// reply edits the message a callback came from, or sends a new one for commands.
func reply(c telebot.Context, edit bool, text string, markup *telebot.ReplyMarkup) error {
	if edit {
		return c.Edit(text, markup)
	}
	return c.Send(text, markup)
}
