package telegram

import "testing"

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		amount int64
		want   string
	}{
		{amount: 10000, want: "100.00 грн"},
		{amount: 10050, want: "100.50 грн"},
		{amount: 5, want: "0.05 грн"},
		{amount: -125, want: "-1.25 грн"},
	}
	for _, test := range tests {
		if got := formatMoney(test.amount); got != test.want {
			t.Errorf("formatMoney(%d) = %q, want %q", test.amount, got, test.want)
		}
	}
}
