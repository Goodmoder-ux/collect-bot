package service

import (
	"strings"
	"unicode"
)

func NormalizeUkrainianText(value string) string {
	var builder strings.Builder
	spacePending := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if spacePending && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(r)
			spacePending = false
			continue
		}
		spacePending = true
	}
	return strings.TrimSpace(builder.String())
}

func studentMatchesComment(fullName, comment string) bool {
	name := NormalizeUkrainianText(fullName)
	text := NormalizeUkrainianText(comment)
	if name == "" || text == "" {
		return false
	}
	if containsTokenSequence(text, name) {
		return true
	}
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return false
	}
	surname := parts[0]
	for _, form := range surnameForms(surname) {
		if containsTokenSequence(text, form) {
			return true
		}
	}
	return false
}

func containsTokenSequence(text, phrase string) bool {
	return strings.Contains(" "+text+" ", " "+phrase+" ")
}

// surnameForms returns the surname and the case forms people use in payment
// comments ("від Петренка", "за Ковальської"). The forms are a deliberate
// over-approximation: a form that also fits another student makes the payment
// ambiguous, which sends it to manual review rather than to the wrong person.
func surnameForms(surname string) []string {
	forms := []string{surname}
	add := func(suffix string, replacements ...string) bool {
		if !strings.HasSuffix(surname, suffix) {
			return false
		}
		stem := strings.TrimSuffix(surname, suffix)
		if stem == "" {
			return false
		}
		for _, replacement := range replacements {
			forms = append(forms, stem+replacement)
		}
		return true
	}
	runes := []rune(surname)
	if len(runes) == 0 {
		return nil
	}
	last := runes[len(runes)-1]
	switch {
	// Adjective-like surnames: Ковальський → Ковальського, Ковальська → Ковальської.
	case add("ий", "ого"), add("ій", "ього", "ія"):
	case add("ька", "ької", "ьку"):
	case add("ова", "ової", "ову"), add("ева", "евої", "еву"), add("єва", "євої", "єву"):
	case add("іна", "іної", "іну"), add("їна", "їної", "їну"), add("ина", "иної", "ину"):
	// Noun-like surnames: Петренко → Петренка, Сорока → Сороки, Кравець → Кравця.
	case add("о", "а"):
	case add("а", "и"), add("я", "і"):
	case add("ець", "ця"):
	case add("ь", "я"), add("й", "я"):
	case strings.ContainsRune(ukrainianConsonants, last):
		forms = append(forms, surname+"а")
	}
	return forms
}

const ukrainianConsonants = "бвгґджзклмнпрстфхцчшщ"
