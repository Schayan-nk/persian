package format

import (
	"strings"
)

// Number-to-words rendering.
//
// Iranian purchase orders, invoices and contracts almost always repeat the
// amount in words ("مبلغ به حروف") next to the figure, so this is a first-class
// feature rather than a curiosity. Both converters work on the digit string
// directly: a rial amount can exceed 2^53 and must not be routed through
// float64.

var (
	faOnes  = [...]string{"", "یک", "دو", "سه", "چهار", "پنج", "شش", "هفت", "هشت", "نه"}
	faTeen  = [...]string{"ده", "یازده", "دوازده", "سیزده", "چهارده", "پانزده", "شانزده", "هفده", "هجده", "نوزده"}
	faTens  = [...]string{"", "", "بیست", "سی", "چهل", "پنجاه", "شصت", "هفتاد", "هشتاد", "نود"}
	faHund  = [...]string{"", "صد", "دویست", "سیصد", "چهارصد", "پانصد", "ششصد", "هفتصد", "هشتصد", "نهصد"}
	faScale = [...]string{"", "هزار", "میلیون", "میلیارد", "بیلیون", "بیلیارد"}
)

// NumberToPersianWords spells out an integer amount in Persian. A fractional
// part is announced with "ممیز"; non-numeric input is returned unchanged.
func NumberToPersianWords(s string) string {
	neg, intPart, fracPart, ok := splitNumeric(s)
	if !ok {
		return s
	}
	out := faIntWords(intPart)
	if fracPart != "" {
		out += " ممیز " + faIntWords(fracPart)
	}
	if neg {
		out = "منفی " + out
	}
	return out
}

func faIntWords(digits string) string {
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "صفر"
	}
	groups := splitGroups(digits)
	if len(groups) > len(faScale) {
		return digits // beyond بیلیارد we would be inventing vocabulary
	}
	var parts []string
	for i, g := range groups {
		if g == 0 {
			continue
		}
		scale := faScale[len(groups)-1-i]
		w := faTriple(g)
		if scale != "" {
			w += " " + scale
		}
		parts = append(parts, w)
	}
	if len(parts) == 0 {
		return "صفر"
	}
	return strings.Join(parts, " و ")
}

func faTriple(n int) string {
	var parts []string
	if h := n / 100; h > 0 {
		parts = append(parts, faHund[h])
	}
	r := n % 100
	switch {
	case r == 0:
	case r < 10:
		parts = append(parts, faOnes[r])
	case r < 20:
		parts = append(parts, faTeen[r-10])
	default:
		if u := r % 10; u > 0 {
			parts = append(parts, faTens[r/10], faOnes[u])
		} else {
			parts = append(parts, faTens[r/10])
		}
	}
	return strings.Join(parts, " و ")
}

var (
	enOnes  = [...]string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
	enTeen  = [...]string{"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	enTens  = [...]string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	enScale = [...]string{"", "thousand", "million", "billion", "trillion", "quadrillion"}
)

// NumberToEnglishWords spells out an integer amount in English.
func NumberToEnglishWords(s string) string {
	neg, intPart, fracPart, ok := splitNumeric(s)
	if !ok {
		return s
	}
	out := enIntWords(intPart)
	if fracPart != "" {
		out += " point " + spellDigits(fracPart)
	}
	if neg {
		out = "minus " + out
	}
	return out
}

func enIntWords(digits string) string {
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "zero"
	}
	groups := splitGroups(digits)
	if len(groups) > len(enScale) {
		return digits
	}
	var parts []string
	for i, g := range groups {
		if g == 0 {
			continue
		}
		w := enTriple(g)
		if scale := enScale[len(groups)-1-i]; scale != "" {
			w += " " + scale
		}
		parts = append(parts, w)
	}
	if len(parts) == 0 {
		return "zero"
	}
	return strings.Join(parts, " ")
}

func enTriple(n int) string {
	var parts []string
	if h := n / 100; h > 0 {
		parts = append(parts, enOnes[h], "hundred")
	}
	r := n % 100
	switch {
	case r == 0:
	case r < 10:
		parts = append(parts, enOnes[r])
	case r < 20:
		parts = append(parts, enTeen[r-10])
	default:
		if u := r % 10; u > 0 {
			parts = append(parts, enTens[r/10]+"-"+enOnes[u])
		} else {
			parts = append(parts, enTens[r/10])
		}
	}
	return strings.Join(parts, " ")
}

func spellDigits(s string) string {
	words := make([]string, 0, len(s))
	for _, c := range s {
		if c == '0' {
			words = append(words, "zero")
			continue
		}
		if c >= '1' && c <= '9' {
			words = append(words, enOnes[c-'0'])
		}
	}
	return strings.Join(words, " ")
}

// splitGroups converts a digit string into 3-digit groups, most significant
// first: "1250000" -> [1, 250, 0].
func splitGroups(digits string) []int {
	var groups []int
	for end := len(digits); end > 0; end -= 3 {
		start := end - 3
		if start < 0 {
			start = 0
		}
		n := 0
		for _, c := range digits[start:end] {
			n = n*10 + int(c-'0')
		}
		groups = append([]int{n}, groups...)
	}
	return groups
}

func splitNumeric(s string) (neg bool, intPart, fracPart string, ok bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return false, "", "", false
	}
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}
	intPart, fracPart, _ = strings.Cut(s, ".")
	fracPart = strings.TrimRight(fracPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) || (fracPart != "" && !isDigits(fracPart)) {
		return false, "", "", false
	}
	return neg, intPart, fracPart, true
}
