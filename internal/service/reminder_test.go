package service

import "testing"

func TestJarURLText(t *testing.T) {
	if got := jarURLText("https://example.com/jar"); got != "https://example.com/jar" {
		t.Fatalf("jarURLText() = %q", got)
	}
	if got := jarURLText(""); got != "Відкрийте меню та оберіть /jar." {
		t.Fatalf("jarURLText() = %q", got)
	}
}
