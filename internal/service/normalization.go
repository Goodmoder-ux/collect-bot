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

func surnameForms(surname string) []string {
	forms := []string{surname}
	switch {
	case strings.HasSuffix(surname, "ко"):
		forms = append(forms, strings.TrimSuffix(surname, "ко")+"ка")
	case strings.HasSuffix(surname, "цький"):
		forms = append(forms, strings.TrimSuffix(surname, "цький")+"цького")
	case strings.HasSuffix(surname, "ський"):
		forms = append(forms, strings.TrimSuffix(surname, "ський")+"ського")
	}
	return forms
}
