package monobank

import "testing"

func TestTransactionComment(t *testing.T) {
	tests := []struct{ comment, description, want string }{
		{"Петренко", "Від: Марія П.", "Петренко · Від: Марія П."},
		{"", "Від: Іван Петренко", "Від: Іван Петренко"},
		{"Петренко", "", "Петренко"},
		{" Петренко ", "Петренко", "Петренко"},
		{"", "", ""},
	}
	for _, test := range tests {
		if got := transactionComment(test.comment, test.description); got != test.want {
			t.Fatalf("transactionComment(%q, %q) = %q, want %q", test.comment, test.description, got, test.want)
		}
	}
}
