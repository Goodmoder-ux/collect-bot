package telegram

import "fmt"

func formatMoneyValue(amountKopiykas int64) string {
	sign := ""
	if amountKopiykas < 0 {
		sign = "-"
		amountKopiykas = -amountKopiykas
	}
	return fmt.Sprintf("%s%d.%02d", sign, amountKopiykas/100, amountKopiykas%100)
}

func formatMoney(amountKopiykas int64) string {
	return formatMoneyValue(amountKopiykas) + " грн"
}
