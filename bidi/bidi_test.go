package bidi

import "testing"

func TestSplitMixedContent(t *testing.T) {
	segs := Split("شرکت نمونه Example Co. تهران", Neutral)
	if len(segs) == 0 {
		t.Fatal("expected segments")
	}
	if segs[0].Dir != RTL {
		t.Errorf("expected first segment RTL, got %v: %q", segs[0].Dir, segs[0].Text)
	}
	var sawLTR bool
	for _, s := range segs {
		if s.Dir == LTR {
			sawLTR = true
		}
	}
	if !sawLTR {
		t.Errorf("expected an LTR segment for the English fragment")
	}
}

func TestSplitPureEnglish(t *testing.T) {
	segs := Split("Example Company", Neutral)
	if len(segs) != 1 || segs[0].Dir != LTR {
		t.Errorf("expected single LTR segment, got %+v", segs)
	}
}

func TestSplitPurePersian(t *testing.T) {
	segs := Split("این یک متن فارسی است", Neutral)
	if len(segs) != 1 || segs[0].Dir != RTL {
		t.Errorf("expected single RTL segment, got %+v", segs)
	}
}

func TestToPersianDigits(t *testing.T) {
	got := ToPersianDigits("Amount: 1250000")
	want := "Amount: ۱۲۵۰۰۰۰"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestToLatinDigits(t *testing.T) {
	got := ToLatinDigits("۱۲۳۴۵")
	if got != "12345" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizePersian(t *testing.T) {
	got := NormalizePersian("علي") // Arabic yeh
	want := "علی"                  // Persian yeh
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsMixed(t *testing.T) {
	if !IsMixed("Hello سلام") {
		t.Error("expected mixed content detected")
	}
	if IsMixed("Hello World") {
		t.Error("did not expect pure English flagged as mixed")
	}
}

func TestHasMultipleNumberGroups(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"+603 4819 0435", true},  // phone number, three groups
		{"060 - 3481 9043", true}, // hyphen-separated, still three digit-bearing groups
		{"1,250,000", false},      // one continuous number, no internal whitespace
		{"10% ", false},           // single group
		{" - ", false},            // no digits at all
		{"", false},
		{"2026 09", true}, // a date is just as susceptible
	} {
		if got := HasMultipleNumberGroups(c.in); got != c.want {
			t.Errorf("HasMultipleNumberGroups(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
