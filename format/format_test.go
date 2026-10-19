package format

import (
	"encoding/json"
	"testing"
)

func TestFormatNumberGrouping(t *testing.T) {
	opt := DefaultOptions()
	cases := map[string]string{
		"1250000":     "1,250,000",
		"1000":        "1,000",
		"999":         "999",
		"-1250000":    "-1,250,000",
		"1234567.891": "1,234,567.891",
	}
	for in, want := range cases {
		got := formatNumberString(in, opt, -1)
		if got != want {
			t.Errorf("formatNumberString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatNumberDecimals(t *testing.T) {
	opt := DefaultOptions()
	got := formatNumberString("1250000", opt, 2)
	if got != "1,250,000.00" {
		t.Errorf("got %q", got)
	}
	got = formatNumberString("1250000.456", opt, 2)
	if got != "1,250,000.46" { // rounds
		t.Errorf("got %q", got)
	}
}

func TestNumberToPersianWords(t *testing.T) {
	cases := map[string]string{
		"0":       "صفر",
		"1":       "یک",
		"21":      "بیست و یک",
		"100":     "صد",
		"1250000": "یک میلیون و دویست و پنجاه هزار",
	}
	for in, want := range cases {
		got := NumberToPersianWords(in)
		if got != want {
			t.Errorf("NumberToPersianWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNumberToEnglishWords(t *testing.T) {
	cases := map[string]string{
		"0":       "zero",
		"1":       "one",
		"21":      "twenty-one",
		"100":     "one hundred",
		"1250000": "one million two hundred fifty thousand",
	}
	for in, want := range cases {
		got := NumberToEnglishWords(in)
		if got != want {
			t.Errorf("NumberToEnglishWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTimeJalali(t *testing.T) {
	tm, ok := ParseTime("1405/05/25", nil)
	if !ok {
		t.Fatal("expected to parse Jalali date")
	}
	if tm.Year() != 2026 {
		t.Errorf("expected Gregorian year 2026, got %d", tm.Year())
	}
}

func TestJalaliRoundTrip(t *testing.T) {
	gy, gm, gd := 2026, 8, 16
	jy, jm, jd := ToJalali(gy, gm, gd)
	bgy, bgm, bgd := FromJalali(jy, jm, jd)
	if bgy != gy || bgm != gm || bgd != gd {
		t.Errorf("round trip mismatch: got %d-%d-%d, want %d-%d-%d", bgy, bgm, bgd, gy, gm, gd)
	}
}

func TestValueBool(t *testing.T) {
	opt := DefaultOptions()
	if got := Value(true, nil, opt); got != "Yes" {
		t.Errorf("got %q", got)
	}
	if got := Value(false, nil, opt); got != "No" {
		t.Errorf("got %q", got)
	}
}

func TestValueDateFilter(t *testing.T) {
	opt := DefaultOptions()
	filters := ParseFilters("date:YYYY-MM-DD")
	got := Value("2026-08-16", filters, opt)
	if got != "2026-08-16" {
		t.Errorf("got %q", got)
	}
}

func TestValueJDateFilter(t *testing.T) {
	opt := DefaultOptions()
	filters := ParseFilters("jdate:YYYY/MM/DD")
	got := Value("2026-08-16", filters, opt)
	if got == "" || got == "2026-08-16" {
		t.Errorf("expected a converted Jalali date, got %q", got)
	}
}

func TestValueNonNumericPassesThrough(t *testing.T) {
	opt := DefaultOptions()
	got := Value("AB-1200", ParseFilters("number"), opt)
	if got != "AB-1200" {
		t.Errorf("expected non-numeric string untouched, got %q", got)
	}
}

func TestValueJSONNumberLargeInt(t *testing.T) {
	opt := DefaultOptions()
	// A value beyond float64's exact integer range must not lose precision.
	n := json.Number("123456789012345678")
	got := Value(n, nil, opt)
	if got != "123,456,789,012,345,678" {
		t.Errorf("got %q", got)
	}
}
