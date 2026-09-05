package docs

import "fmt"

var (
	ones = []string{"", "один", "два", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять"}
	onesFem = []string{"", "одна", "две", "три", "четыре", "пять", "шесть", "семь", "восемь", "девять"}
	teens = []string{"десять", "одиннадцать", "двенадцать", "тринадцать", "четырнадцать", "пятнадцать", "шестнадцать", "семнадцать", "восемнадцать", "девятнадцать"}
	tens = []string{"", "", "двадцать", "тридцать", "сорок", "пятьдесят", "шестьдесят", "семьдесят", "восемьдесят", "девяносто"}
	hundreds = []string{"", "сто", "двести", "триста", "четыреста", "пятьсот", "шестьсот", "семьсот", "восемьсот", "девятьсот"}
)

// RublesInWords пишет сумму прописью для счёта и договора.
func RublesInWords(minor int64) string {
	if minor < 0 {
		minor = 0
	}
	rub := minor / 100
	kop := minor % 100
	return fmt.Sprintf("%s %s %02d коп.", triadWords(rub, true), rubUnit(rub), kop)
}

func triadWords(n int64, feminine bool) string {
	if n == 0 {
		return "ноль"
	}
	parts := make([]string, 0, 8)
	billions := n / 1_000_000_000
	n %= 1_000_000_000
	millions := n / 1_000_000
	n %= 1_000_000
	thousands := n / 1_000
	n %= 1_000

	if billions > 0 {
		parts = append(parts, triad(billions, false), scaleUnit(billions, "миллиард", "миллиарда", "миллиардов"))
	}
	if millions > 0 {
		parts = append(parts, triad(millions, false), scaleUnit(millions, "миллион", "миллиона", "миллионов"))
	}
	if thousands > 0 {
		parts = append(parts, triad(thousands, true), scaleUnit(thousands, "тысяча", "тысячи", "тысяч"))
	}
	if n > 0 {
		parts = append(parts, triad(n, feminine))
	}
	out := ""
	for i, part := range parts {
		if part == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += part
		_ = i
	}
	return out
}

func triad(n int64, feminine bool) string {
	h := n / 100
	n %= 100
	parts := make([]string, 0, 3)
	if h > 0 {
		parts = append(parts, hundreds[h])
	}
	switch {
	case n >= 10 && n <= 19:
		parts = append(parts, teens[n-10])
	default:
		t := n / 10
		o := n % 10
		if t > 0 {
			parts = append(parts, tens[t])
		}
		if o > 0 {
			if feminine {
				parts = append(parts, onesFem[o])
			} else {
				parts = append(parts, ones[o])
			}
		}
	}
	out := ""
	for _, part := range parts {
		if out != "" {
			out += " "
		}
		out += part
	}
	return out
}

func scaleUnit(n int64, one, few, many string) string {
	return pickUnit(n, one, few, many)
}

func rubUnit(n int64) string {
	return pickUnit(n, "рубль", "рубля", "рублей")
}

func pickUnit(n int64, one, few, many string) string {
	n = n % 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
