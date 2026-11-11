package telegram

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

type fakeCollectionRepo struct {
	active []domain.Collection
	closed []domain.Collection
}

func (f *fakeCollectionRepo) ListActive(context.Context) ([]domain.Collection, error) {
	return f.active, nil
}

func (f *fakeCollectionRepo) ListClosed(_ context.Context, limit int) ([]domain.Collection, error) {
	return f.closed[:min(limit, len(f.closed))], nil
}

func (f *fakeCollectionRepo) GetByID(_ context.Context, id int64) (domain.Collection, error) {
	for _, collection := range slices.Concat(f.active, f.closed) {
		if collection.ID == id {
			return collection, nil
		}
	}
	return domain.Collection{}, sql.ErrNoRows
}

func (f *fakeCollectionRepo) CloseIfAllStudentsPaid(context.Context, int64) (bool, error) {
	return false, nil
}

func (f *fakeCollectionRepo) Create(context.Context, string, int64, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeCollectionRepo) CloseManually(context.Context, int64) error { return nil }

func testCollections() *fakeCollectionRepo {
	closedAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	reason := "all_paid"
	return &fakeCollectionRepo{
		active: []domain.Collection{
			{ID: 3, Title: "Квіти", Amount: 15000, Status: "active", DeadlineAt: time.Date(2026, 11, 20, 23, 59, 59, 0, time.UTC)},
			{ID: 4, Title: "Екскурсія", Amount: 40000, Status: "active", DeadlineAt: time.Date(2026, 12, 1, 23, 59, 59, 0, time.UTC)},
		},
		closed: []domain.Collection{
			{ID: 1, Title: "Подарунок", Amount: 10000, Status: "closed", ClosedAt: &closedAt, ClosedReason: &reason},
		},
	}
}

func TestCollectionPickerListsActiveCollections(t *testing.T) {
	c := &fakeContext{}
	if err := renderCollectionPicker(c, testCollections(), pickExport, false, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"pick_collection|export|3", "pick_collection|export|4", "pick_archive|export"}
	if got := c.buttons(); !slices.Equal(got, want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
	if len(c.sent) != 1 || len(c.edited) != 0 {
		t.Fatalf("a command must get a new message: sent=%d edited=%d", len(c.sent), len(c.edited))
	}
	if label := c.markup.InlineKeyboard[0][0].Text; label != "Квіти — 150.00 грн, до 20.11" {
		t.Fatalf("label = %q", label)
	}
}

func TestCollectionPickerArchiveEditsMessage(t *testing.T) {
	c := &fakeContext{}
	if err := renderCollectionPicker(c, testCollections(), pickStatus, true, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"pick_collection|status|1", "pick_active|status"}
	if got := c.buttons(); !slices.Equal(got, want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
	if len(c.edited) != 1 || len(c.sent) != 0 {
		t.Fatalf("a button press must edit its message: sent=%d edited=%d", len(c.sent), len(c.edited))
	}
	if label := c.markup.InlineKeyboard[0][0].Text; !strings.HasPrefix(label, "Подарунок — закрито 05.11.2026") {
		t.Fatalf("label = %q", label)
	}
}

func TestCollectionPickerForCloseHasNoArchive(t *testing.T) {
	c := &fakeContext{}
	if err := renderCollectionPicker(c, testCollections(), pickClose, false, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"pick_collection|close|3", "pick_collection|close|4"}
	if got := c.buttons(); !slices.Equal(got, want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
}

func TestCollectionPickerEmptyStates(t *testing.T) {
	empty := &fakeCollectionRepo{}
	c := &fakeContext{}
	if err := renderCollectionPicker(c, empty, pickStatus, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.sent[0], "Активних зборів зараз немає") {
		t.Fatalf("text = %q", c.sent[0])
	}
	if got := c.buttons(); !slices.Equal(got, []string{"pick_archive|status"}) {
		t.Fatalf("buttons = %v, want only the archive button", got)
	}
	c = &fakeContext{}
	if err := renderCollectionPicker(c, empty, pickStatus, true, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.edited[0], "Закритих зборів ще немає") {
		t.Fatalf("text = %q", c.edited[0])
	}
}

func TestParsePickData(t *testing.T) {
	action, id, err := parsePickData("close|7")
	if err != nil || action != pickClose || id != 7 {
		t.Fatalf("parsePickData = %q, %d, %v", action, id, err)
	}
	for _, data := range []string{"", "status", "delete|1", "status|0", "status|x"} {
		if _, _, err := parsePickData(data); err == nil {
			t.Fatalf("parsePickData(%q) succeeded, want error", data)
		}
	}
}

func TestOptionalIDArgument(t *testing.T) {
	if _, given, err := optionalIDArgument("/status"); err != nil || given {
		t.Fatalf("no argument: given=%v err=%v", given, err)
	}
	if id, given, err := optionalIDArgument("/status 12"); err != nil || !given || id != 12 {
		t.Fatalf("with ID: id=%d given=%v err=%v", id, given, err)
	}
	for _, text := range []string{"/status abc", "/status 0", "/status -3", "/status 1 2"} {
		_, _, err := optionalIDArgument(text)
		var ue *userError
		if !errors.As(err, &ue) {
			t.Fatalf("optionalIDArgument(%q) err = %v, want a user-facing error", text, err)
		}
	}
}

func TestConfirmCloseCollection(t *testing.T) {
	repo := testCollections()
	c := &fakeContext{}
	if err := confirmCloseCollection(c, repo, 3, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.edited[0], "«Квіти»") {
		t.Fatalf("confirmation does not name the collection: %q", c.edited[0])
	}
	want := []string{"close_collection_confirm|3", "close_collection_cancel|3"}
	if got := c.buttons(); !slices.Equal(got, want) {
		t.Fatalf("buttons = %v, want %v", got, want)
	}
	if err := confirmCloseCollection(&fakeContext{}, repo, 1, false); !errors.Is(err, domain.ErrCollectionNotActive) {
		t.Fatalf("closing a closed collection: err = %v, want ErrCollectionNotActive", err)
	}
	if err := confirmCloseCollection(&fakeContext{}, repo, 99, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("closing a missing collection: err = %v, want sql.ErrNoRows", err)
	}
}

func TestStatusMarkup(t *testing.T) {
	c := &fakeContext{markup: statusMarkup(5, true)}
	want := []string{"status_remind|5", "status_cash|5", "status_export|5", "status_close|5", "status_back_collections|"}
	if got := c.buttons(); !slices.Equal(got, want) {
		t.Fatalf("active buttons = %v, want %v", got, want)
	}
	c = &fakeContext{markup: statusMarkup(5, false)}
	want = []string{"status_export|5", "pick_archive|status"}
	if got := c.buttons(); !slices.Equal(got, want) {
		t.Fatalf("closed buttons = %v, want %v", got, want)
	}
}
