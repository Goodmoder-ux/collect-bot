package telegram

import (
	"testing"

	"gopkg.in/telebot.v3"
)

func TestParseHryvnia(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{name: "whole amount", input: "100", want: 10000},
		{name: "decimal point", input: "100.50", want: 10050},
		{name: "decimal comma", input: "100,5", want: 10050},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseHryvnia(test.input)
			if err != nil {
				t.Fatalf("parseHryvnia(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("parseHryvnia(%q) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestParseHryvniaRejectsInvalidAmounts(t *testing.T) {
	for _, input := range []string{"", "0", "-1", "1.234", "one"} {
		if _, err := parseHryvnia(input); err == nil {
			t.Fatalf("parseHryvnia(%q) succeeded, want error", input)
		}
	}
}

func TestTelegramDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		user     *telebot.User
		expected string
	}{
		{name: "full name", user: &telebot.User{FirstName: "Іван", LastName: "Петренко"}, expected: "Іван Петренко"},
		{name: "first name only", user: &telebot.User{FirstName: "Іван"}, expected: "Іван"},
		{name: "missing name", user: &telebot.User{}, expected: "не вказано"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramDisplayName(test.user); got != test.expected {
				t.Fatalf("telegramDisplayName() = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestTelegramUsername(t *testing.T) {
	if got := telegramUsername(&telebot.User{Username: "ivan"}); got != "@ivan" {
		t.Fatalf("telegramUsername() = %q, want %q", got, "@ivan")
	}
	if got := telegramUsername(&telebot.User{}); got != "відсутній" {
		t.Fatalf("telegramUsername() = %q, want %q", got, "відсутній")
	}
}
