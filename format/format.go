// Package format turns JSON values from ProcessMaker into the strings that go
// into a document, and implements the placeholder filter pipeline.
package format

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Schayan-nk/persian/bidi"
)

// titleCase capitalises the first letter of every word: a letter that follows
// anything but a letter, a digit, an underscore or an apostrophe. What the
// deprecated strings.Title did for the headings this filter is used on,
// without its trouble with Unicode punctuation -- and "don't" stays "Don't"
// where strings.Title made it "Don'T".
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		start := !unicode.IsLetter(prev) && !unicode.IsDigit(prev) && prev != '_' && prev != '\''
		prev = r
		if start {
			return unicode.ToTitle(r)
		}
		return r
	}, s)
}

// Options configures rendering; it is derived from the service configuration.
type Options struct {
	ThousandSep string
	DecimalSep  string
	Decimals    int // -1 keeps the decimals the caller supplied
	BoolTrue    string
	BoolFalse   string
	DateFormat  string // default pattern for the `date` filter
	JDateFormat string // default pattern for the `jdate` filter
	Location    *time.Location
	MaxRunes    int
}

// DefaultOptions returns sensible values for a bilingual Persian/English site.
func DefaultOptions() Options {
	return Options{
		ThousandSep: ",",
		DecimalSep:  ".",
		Decimals:    -1,
		BoolTrue:    "Yes",
		BoolFalse:   "No",
		DateFormat:  "YYYY-MM-DD",
		JDateFormat: "YYYY/MM/DD",
		Location:    time.Local,
		MaxRunes:    100000,
	}
}

// Filter is one stage of a placeholder's `|` pipeline.
type Filter struct {
	Name string
	Args []string
}

// ParseFilters parses the text after the first `|` of a placeholder.
// Example input: `number:2|fa`.
func ParseFilters(spec string) []Filter {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	var out []Filter
	for _, part := range strings.Split(spec, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, argstr, _ := strings.Cut(part, ":")
		f := Filter{Name: strings.ToLower(strings.TrimSpace(name))}
		if argstr != "" {
			for _, a := range strings.Split(argstr, ",") {
				f.Args = append(f.Args, strings.TrimSpace(a))
			}
		}
		out = append(out, f)
	}
	return out
}

// DirectiveOf reports a direction override requested by the filter chain
// (`|rtl` or `|ltr`), which the renderer applies to the generated runs.
func DirectiveOf(filters []Filter) bidi.Direction {
	for _, f := range filters {
		switch f.Name {
		case "rtl":
			return bidi.RTL
		case "ltr":
			return bidi.LTR
		}
	}
	return bidi.Neutral
}

// Value renders a decoded JSON value to its document string, then applies the
// filter chain.
func Value(v any, filters []Filter, opt Options) string {
	s := stringify(v, opt)
	for _, f := range filters {
		s = apply(s, v, f, opt)
	}
	if opt.MaxRunes > 0 {
		if r := []rune(s); len(r) > opt.MaxRunes {
			s = string(r[:opt.MaxRunes])
		}
	}
	return s
}

func stringify(v any, opt Options) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return formatNumberString(x.String(), opt, opt.Decimals)
	case bool:
		if x {
			return opt.BoolTrue
		}
		return opt.BoolFalse
	case float64:
		return formatNumberString(strconv.FormatFloat(x, 'f', -1, 64), opt, opt.Decimals)
	case int:
		return formatNumberString(strconv.Itoa(x), opt, opt.Decimals)
	case int64:
		return formatNumberString(strconv.FormatInt(x, 10), opt, opt.Decimals)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, stringify(e, opt))
		}
		return strings.Join(parts, "، ")
	case map[string]any:
		// A nested object reaching a leaf placeholder is a caller mistake; show
		// something readable rather than Go's map syntax.
		parts := make([]string, 0, len(x))
		for k, e := range x {
			parts = append(parts, k+": "+stringify(e, opt))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(x)
	}
}

func apply(s string, orig any, f Filter, opt Options) string {
	switch f.Name {
	case "raw":
		return rawString(orig)

	case "number", "num":
		d := opt.Decimals
		if len(f.Args) > 0 {
			if n, err := strconv.Atoi(f.Args[0]); err == nil {
				d = n
			}
		}
		return formatNumberString(bidi.ToLatinDigits(rawString(orig)), opt, d)

	case "int":
		return formatNumberString(bidi.ToLatinDigits(rawString(orig)), opt, 0)

	case "money":
		out := formatNumberString(bidi.ToLatinDigits(rawString(orig)), opt, 0)
		if len(f.Args) > 0 && f.Args[0] != "" {
			out += " " + f.Args[0]
		}
		return out

	case "percent":
		d := 0
		if len(f.Args) > 0 {
			if n, err := strconv.Atoi(f.Args[0]); err == nil {
				d = n
			}
		}
		return formatNumberString(bidi.ToLatinDigits(rawString(orig)), opt, d) + "%"

	case "fa", "fadigits":
		return bidi.ToPersianDigits(s)

	case "en", "endigits":
		return bidi.ToLatinDigits(s)

	case "fawords":
		return NumberToPersianWords(bidi.ToLatinDigits(rawString(orig)))

	case "enwords":
		return NumberToEnglishWords(bidi.ToLatinDigits(rawString(orig)))

	case "upper":
		return strings.ToUpper(s)

	case "lower":
		return strings.ToLower(s)

	case "title":
		return titleCase(s)

	case "trim":
		return strings.TrimSpace(s)

	case "nowrap":
		return strings.ReplaceAll(s, " ", " ")

	case "normalize", "nfa":
		return bidi.NormalizePersian(s)

	case "date":
		pattern := opt.DateFormat
		if len(f.Args) > 0 && f.Args[0] != "" {
			pattern = strings.Join(f.Args, ",")
		}
		return formatDate(rawString(orig), pattern, false, opt)

	case "jdate", "jalali":
		pattern := opt.JDateFormat
		if len(f.Args) > 0 && f.Args[0] != "" {
			pattern = strings.Join(f.Args, ",")
		}
		return formatDate(rawString(orig), pattern, true, opt)

	case "bool":
		t, fa := opt.BoolTrue, opt.BoolFalse
		if len(f.Args) > 0 {
			t = f.Args[0]
		}
		if len(f.Args) > 1 {
			fa = f.Args[1]
		}
		if truthy(orig) {
			return t
		}
		return fa

	case "default":
		if strings.TrimSpace(s) == "" && len(f.Args) > 0 {
			return strings.Join(f.Args, ",")
		}
		return s

	case "rtl", "ltr":
		// Direction hints are consumed by the renderer, not the string.
		return s

	default:
		// An unknown filter is a template typo, not a reason to fail a whole
		// document; the value passes through unchanged.
		return s
	}
}

// rawString renders a value without grouping, for filters that re-parse it.
func rawString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case json.Number:
		f, err := x.Float64()
		return err == nil && f != 0
	case float64:
		return x != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "", "0", "false", "no", "off", "خیر", "نه":
			return false
		}
		return true
	default:
		return true
	}
}

// formatNumberString groups the integer part of a numeric string. Input that is
// not numeric is returned untouched -- a part number like "AB-1200" must not be
// mangled into "AB-1,200".
func formatNumberString(s string, opt Options, decimals int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	neg := false
	body := s
	switch body[0] {
	case '-':
		neg, body = true, body[1:]
	case '+':
		body = body[1:]
	}
	intPart, fracPart, hasFrac := strings.Cut(body, ".")
	if intPart == "" && !hasFrac {
		return s
	}
	if !isDigits(intPart) || (hasFrac && !isDigits(fracPart)) {
		return s
	}
	if intPart == "" {
		intPart = "0"
	}

	if decimals >= 0 {
		rounded, err := roundDecimalString(intPart, fracPart, decimals)
		if err != nil {
			return s
		}
		intPart, fracPart = rounded[0], rounded[1]
		hasFrac = decimals > 0
	} else {
		fracPart = strings.TrimRight(fracPart, "0")
		hasFrac = fracPart != ""
	}

	var b strings.Builder
	if neg && (strings.Trim(intPart, "0") != "" || strings.Trim(fracPart, "0") != "") {
		b.WriteByte('-')
	}
	b.WriteString(group(intPart, opt.ThousandSep))
	if hasFrac && fracPart != "" {
		b.WriteString(opt.DecimalSep)
		b.WriteString(fracPart)
	}
	return b.String()
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func group(intPart, sep string) string {
	if sep == "" || len(intPart) <= 3 {
		return intPart
	}
	var b strings.Builder
	lead := len(intPart) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(intPart[:lead])
	for i := lead; i < len(intPart); i += 3 {
		b.WriteString(sep)
		b.WriteString(intPart[i : i+3])
	}
	return b.String()
}

// roundDecimalString rounds a decimal held as two digit strings, half away from
// zero, without going through float64. Amounts in rial routinely exceed 2^53,
// and silently losing the last digits of a contract value is unacceptable.
func roundDecimalString(intPart, fracPart string, decimals int) ([2]string, error) {
	if decimals < 0 || decimals > 18 {
		return [2]string{}, fmt.Errorf("format: unsupported decimal count %d", decimals)
	}
	if len(fracPart) <= decimals {
		return [2]string{intPart, fracPart + strings.Repeat("0", decimals-len(fracPart))}, nil
	}
	keep := fracPart[:decimals]
	roundUp := fracPart[decimals] >= '5'
	if !roundUp {
		return [2]string{intPart, keep}, nil
	}
	digits := []byte(intPart + keep)
	i := len(digits) - 1
	for ; i >= 0; i-- {
		if digits[i] < '9' {
			digits[i]++
			break
		}
		digits[i] = '0'
	}
	if i < 0 {
		digits = append([]byte{'1'}, digits...)
	}
	split := len(digits) - decimals
	return [2]string{string(digits[:split]), string(digits[split:])}, nil
}

// ParseTime accepts the date shapes ProcessMaker and hand-written triggers
// actually produce, including Jalali dates written as 1405/05/25.
func ParseTime(s string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(bidi.ToLatinDigits(s))
	if s == "" {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.Local
	}

	// Unix timestamp (seconds or milliseconds).
	if isDigits(s) && (len(s) == 10 || len(s) == 13) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			if len(s) == 13 {
				return time.UnixMilli(n).In(loc), true
			}
			return time.Unix(n, 0).In(loc), true
		}
	}

	normalized := strings.ReplaceAll(s, "/", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02-01-2006",
		"20060102",
	}

	// A leading year in the 1200–1700 band can only be Jalali.
	if y, rest, ok := leadingYear(normalized); ok && y >= 1200 && y <= 1700 {
		if t, ok := parseJalaliDate(y, rest, loc); ok {
			return t, true
		}
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, normalized, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func leadingYear(s string) (int, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < 3 || i > 4 {
		return 0, "", false
	}
	y, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, "", false
	}
	return y, strings.TrimPrefix(s[i:], "-"), true
}

func parseJalaliDate(jy int, rest string, loc *time.Location) (time.Time, bool) {
	fields := strings.FieldsFunc(rest, func(r rune) bool {
		return r == '-' || r == ' ' || r == ':' || r == 'T'
	})
	get := func(i int) int {
		if i < len(fields) {
			n, err := strconv.Atoi(fields[i])
			if err == nil {
				return n
			}
		}
		return 0
	}
	jm, jd := get(0), get(1)
	if jm < 1 || jm > 12 || jd < 1 || jd > 31 {
		return time.Time{}, false
	}
	gy, gm, gd := FromJalali(jy, jm, jd)
	h, mi, sec := get(2), get(3), get(4)
	return time.Date(gy, time.Month(gm), gd, h, mi, sec, 0, loc), true
}

func formatDate(raw, pattern string, jalali bool, opt Options) string {
	t, ok := ParseTime(raw, opt.Location)
	if !ok {
		return raw // unparseable input passes through rather than vanishing
	}
	if jalali {
		return formatJalali(t, pattern)
	}
	return formatGregorian(t, pattern)
}

// pattern tokens are the friendly ones an integrator expects (YYYY/MM/DD),
// not Go's reference-time layout, which is confusing to non-Go users.
func formatGregorian(t time.Time, pattern string) string {
	return expandPattern(pattern, map[string]string{
		"YYYY": fmt.Sprintf("%04d", t.Year()),
		"YY":   fmt.Sprintf("%02d", t.Year()%100),
		"MMMM": t.Month().String(),
		"MMM":  t.Format("Jan"),
		"MM":   fmt.Sprintf("%02d", int(t.Month())),
		"M":    strconv.Itoa(int(t.Month())),
		"DD":   fmt.Sprintf("%02d", t.Day()),
		"D":    strconv.Itoa(t.Day()),
		"dddd": t.Weekday().String(),
		"HH":   fmt.Sprintf("%02d", t.Hour()),
		"mm":   fmt.Sprintf("%02d", t.Minute()),
		"ss":   fmt.Sprintf("%02d", t.Second()),
	})
}

func formatJalali(t time.Time, pattern string) string {
	jy, jm, jd, wd := jalaliParts(t)
	return expandPattern(pattern, map[string]string{
		"YYYY": fmt.Sprintf("%04d", jy),
		"YY":   fmt.Sprintf("%02d", jy%100),
		"MMMM": jalaliMonthsFa[jm-1],
		"MMM":  jalaliMonthsEn[jm-1],
		"MM":   fmt.Sprintf("%02d", jm),
		"M":    strconv.Itoa(jm),
		"DD":   fmt.Sprintf("%02d", jd),
		"D":    strconv.Itoa(jd),
		"dddd": jalaliWeekdaysFa[int(wd)],
		"HH":   fmt.Sprintf("%02d", t.Hour()),
		"mm":   fmt.Sprintf("%02d", t.Minute()),
		"ss":   fmt.Sprintf("%02d", t.Second()),
	})
}

// tokens are matched longest-first so YYYY wins over YY and MMMM over MM.
var patternTokens = []string{"YYYY", "MMMM", "dddd", "MMM", "YY", "MM", "DD", "HH", "mm", "ss", "M", "D"}

func expandPattern(pattern string, vals map[string]string) string {
	if pattern == "" {
		pattern = "YYYY-MM-DD"
	}
	var b strings.Builder
	for i := 0; i < len(pattern); {
		matched := false
		for _, tok := range patternTokens {
			if strings.HasPrefix(pattern[i:], tok) {
				b.WriteString(vals[tok])
				i += len(tok)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(pattern[i])
			i++
		}
	}
	return b.String()
}

// Round is a small helper used by the grid total calculations.
func Round(f float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(f*p) / p
}
