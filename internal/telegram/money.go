package telegram

import "fmt"

func formatMoney(amountKopiykas int64) string {
	sign := ""
	if amountKopiykas < 0 {
		sign = "-"
		amountKopiykas = -amountKopiykas
	}
	return fmt.Sprintf("%s%d.%02d грн", sign, amountKopiykas/100, amountKopiykas%100)
}
