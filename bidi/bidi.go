// Package bidi classifies text direction and splits mixed Persian/English
// strings into direction-homogeneous segments.
//
// Why this matters for DOCX: in WordprocessingML the *run* is the unit that
// carries direction. A run marked `<w:rtl/>` is an RTL embedding; one without it
// is LTR. If a single run holds both "شرکت نمونه" and "Example Co." Word has to
// guess, and the result is the classic mangled ordering users see when a value
// is dropped into a template verbatim. So when a value mixes scripts we emit one
// run per direction and let Word's own bidirectional algorithm lay them out.
//
// This is a pragmatic subset of UAX #9: it resolves strong characters, then
// assigns neutral runs (spaces, punctuation, digits) to the surrounding
// direction, falling back to the paragraph base direction when the neighbours
// disagree. Word applies the full algorithm at render time; our only job is to
// mark each run's embedding level correctly.
package bidi

import (
	"strings"
	"unicode"
)

// Direction is the resolved embedding direction of a stretch of text.
type Direction uint8

const (
	Neutral Direction = iota
	LTR
	RTL
)

func (d Direction) String() string {
	switch d {
	case LTR:
		return "ltr"
	case RTL:
		return "rtl"
	default:
		return "neutral"
	}
}

// Segment is a maximal stretch of text with one embedding direction.
type Segment struct {
	Text string
	Dir  Direction
}

// IsRTLRune reports whether r belongs to a right-to-left script.
//
// Covers Arabic and Persian (including the presentation forms Windows fonts
// still emit), Hebrew, Syriac, Thaana and NKo -- everything a ProcessMaker form
// in this part of the world is likely to contain.
func IsRTLRune(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x05FF: // Hebrew
		return true
	case r >= 0x0600 && r <= 0x06FF: // Arabic (incl. Persian letters ی گ چ پ ژ ک)
		return true
	case r >= 0x0700 && r <= 0x074F: // Syriac
		return true
	case r >= 0x0750 && r <= 0x077F: // Arabic Supplement
		return true
	case r >= 0x0780 && r <= 0x07BF: // Thaana
		return true
	case r >= 0x07C0 && r <= 0x08FF: // NKo, Samaritan, Arabic Extended-A
		return true
	case r >= 0xFB1D && r <= 0xFDFF: // Hebrew/Arabic presentation forms A
		return true
	case r >= 0xFE70 && r <= 0xFEFF: // Arabic presentation forms B
		return true
	case r >= 0x10800 && r <= 0x10FFF: // ancient RTL scripts
		return true
	case r >= 0x1E800 && r <= 0x1EFFF: // Adlam, Arabic Mathematical
		return true
	}
	return false
}

// isRTLDigit reports whether r is an Arabic-Indic or Persian digit. These are
// weak: they never establish direction on their own but sit comfortably inside
// an RTL run.
func isRTLDigit(r rune) bool {
	return (r >= 0x0660 && r <= 0x0669) || (r >= 0x06F0 && r <= 0x06F9)
}

func classify(r rune) Direction {
	if IsRTLRune(r) {
		if isRTLDigit(r) {
			return Neutral
		}
		// Arabic diacritics and the tatweel are RTL but not strong; treating
		// them as strong is harmless because they only ever appear beside
		// Arabic-script letters.
		return RTL
	}
	if unicode.IsLetter(r) {
		return LTR
	}
	return Neutral
}

// ContainsRTL reports whether s holds at least one strong RTL character.
func ContainsRTL(s string) bool {
	for _, r := range s {
		if classify(r) == RTL {
			return true
		}
	}
	return false
}

// ContainsLTR reports whether s holds at least one strong LTR character.
func ContainsLTR(s string) bool {
	for _, r := range s {
		if classify(r) == LTR {
			return true
		}
	}
	return false
}

// BaseDirection returns the direction of the first strong character, or
// fallback when the string has none (digits, punctuation, empty).
func BaseDirection(s string, fallback Direction) Direction {
	for _, r := range s {
		if d := classify(r); d != Neutral {
			return d
		}
	}
	return fallback
}

// IsMixed reports whether s contains strong characters of both directions.
func IsMixed(s string) bool { return ContainsRTL(s) && ContainsLTR(s) }

// Unicode bidirectional formatting characters.
//
// Marking runs with w:rtl fixes the ordering *within* a value, but it cannot
// change how the value as a whole is placed inside its paragraph: that is
// decided by the paragraph's base direction. A Persian phrase dropped into an
// LTR paragraph therefore still lays out starting from the left.
//
// Wrapping the value in an isolate makes it one directional unit with its own
// base direction, so it reads correctly without altering the paragraph -- which
// matters because the paragraph's direction is the template author's design
// decision, not ours to overwrite.
const (
	LRI = "⁦" // LEFT-TO-RIGHT ISOLATE
	RLI = "⁧" // RIGHT-TO-LEFT ISOLATE
	PDI = "⁩" // POP DIRECTIONAL ISOLATE

	LRE = "‪" // LEFT-TO-RIGHT EMBEDDING (pre-Unicode 6.3)
	RLE = "‫" // RIGHT-TO-LEFT EMBEDDING
	PDF = "‬" // POP DIRECTIONAL FORMATTING
)

// WrapMode selects which formatting characters Wrap uses.
type WrapMode string

const (
	// WrapIsolate uses U+2066..U+2069. Correct and self-contained: neighbouring
	// text cannot leak into the wrapped value's resolution.
	WrapIsolate WrapMode = "isolate"
	// WrapEmbed uses the older U+202A..U+202C, for Word versions that predate
	// isolate support.
	WrapEmbed WrapMode = "embed"
	// WrapOff adds nothing.
	WrapOff WrapMode = "off"
)

// Wrap makes s a self-contained directional unit reading in dir.
func Wrap(s string, dir Direction, mode WrapMode) string {
	if s == "" || dir == Neutral || mode == WrapOff {
		return s
	}
	switch mode {
	case WrapEmbed:
		if dir == RTL {
			return RLE + s + PDF
		}
		return LRE + s + PDF
	default:
		if dir == RTL {
			return RLI + s + PDI
		}
		return LRI + s + PDI
	}
}

// IsFormattingRune reports whether r is a bidi formatting character, which
// should be ignored when classifying text.
func IsFormattingRune(r rune) bool {
	switch r {
	case 0x2066, 0x2067, 0x2068, 0x2069, 0x202A, 0x202B, 0x202C, 0x202D, 0x202E:
		return true
	}
	return false
}

// StripFormatting removes bidi formatting characters, leaving the text a reader
// actually sees. Used when comparing or measuring content.
func StripFormatting(s string) string {
	if !strings.ContainsAny(s, "⁦⁧⁨⁩‪‫‬‭‮") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !IsFormattingRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HasStrong reports whether s contains any strong directional character.
//
// A fragment without one -- "10% ", "1,250,000", " - " -- has no direction of its
// own. Forcing `w:rtl` (or `w:rtl w:val="0"`) onto such a run overrides the
// contextual resolution Word would otherwise do correctly, which is how a
// percent sign or a thousands separator ends up on the wrong side of a Persian
// phrase. Runs built from these fragments should inherit instead.
func HasStrong(s string) bool {
	for _, r := range s {
		if classify(r) != Neutral {
			return true
		}
	}
	return false
}

// HasMultipleNumberGroups reports whether s contains two or more digit-bearing
// groups separated by whitespace, e.g. a phone number ("+603 4819 0435") or a
// date written with spaces.
//
// Such a value has HasStrong == false -- no letter asserts a direction -- but
// unlike a single grouped number ("1,250,000"), it is not safe to just let it
// inherit an RTL paragraph's direction. A comma-grouped number is one
// continuous "European Number" run as far as Word's bidi algorithm is
// concerned, so there is nothing to reorder. Whitespace between separate
// digit groups is exactly the kind of neutral boundary the algorithm uses to
// split a run into independent islands, and inside an RTL embedding,
// independent LTR islands are laid out right-to-left relative to each other
// -- which is the mangled group order users see ("+603 4819 0435" printing as
// "0435 4819 603+").
func HasMultipleNumberGroups(s string) bool {
	groups := 0
	inGroup, sawDigit := false, false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if inGroup && sawDigit {
				groups++
			}
			inGroup, sawDigit = false, false
			continue
		}
		inGroup = true
		if r >= '0' && r <= '9' {
			sawDigit = true
		}
	}
	if inGroup && sawDigit {
		groups++
	}
	return groups >= 2
}

// Split breaks s into direction-homogeneous segments.
//
// base is the surrounding paragraph direction, used to place neutral runs that
// sit between opposite-direction neighbours (a hyphen between a Persian word and
// an English one, say). A string with no strong characters yields a single
// segment carrying base.
func Split(s string, base Direction) []Segment {
	if s == "" {
		return nil
	}
	if base == Neutral {
		base = BaseDirection(s, LTR)
	}

	runes := []rune(s)
	dirs := make([]Direction, len(runes))
	strong := false
	for i, r := range runes {
		dirs[i] = classify(r)
		if dirs[i] != Neutral {
			strong = true
		}
	}
	if !strong {
		return []Segment{{Text: s, Dir: base}}
	}

	// Resolve each maximal neutral run against its strong neighbours.
	for i := 0; i < len(dirs); {
		if dirs[i] != Neutral {
			i++
			continue
		}
		j := i
		for j < len(dirs) && dirs[j] == Neutral {
			j++
		}
		prev, next := Neutral, Neutral
		if i > 0 {
			prev = dirs[i-1]
		}
		if j < len(dirs) {
			next = dirs[j]
		}
		resolved := base
		switch {
		case prev != Neutral && prev == next:
			resolved = prev
		case prev == Neutral && next != Neutral:
			// Leading neutrals belong with what follows only if that matches
			// the base direction; otherwise they stay on the base side.
			if next == base {
				resolved = next
			}
		case next == Neutral && prev != Neutral:
			// Trailing neutrals: keep them attached to the preceding text so a
			// full stop does not jump to the far edge of the line.
			resolved = prev
		}
		for k := i; k < j; k++ {
			dirs[k] = resolved
		}
		i = j
	}

	var out []Segment
	start := 0
	for i := 1; i <= len(runes); i++ {
		if i == len(runes) || dirs[i] != dirs[start] {
			out = append(out, Segment{Text: string(runes[start:i]), Dir: dirs[start]})
			start = i
		}
	}

	// Merge neighbours that ended up with the same direction (possible after
	// neutral resolution) and drop empty fragments.
	merged := out[:0]
	for _, seg := range out {
		if seg.Text == "" {
			continue
		}
		if n := len(merged); n > 0 && merged[n-1].Dir == seg.Dir {
			merged[n-1].Text += seg.Text
			continue
		}
		merged = append(merged, seg)
	}
	return merged
}

const (
	latinDigits   = "0123456789"
	persianDigits = "۰۱۲۳۴۵۶۷۸۹"
	arabicDigits  = "٠١٢٣٤٥٦٧٨٩"
)

// ToPersianDigits converts Latin digits to Persian (Extended Arabic-Indic)
// digits, leaving everything else alone.
func ToPersianDigits(s string) string {
	if !strings.ContainsAny(s, latinDigits) {
		return s
	}
	fa := []rune(persianDigits)
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(fa[r-'0'])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ToLatinDigits normalises Persian and Arabic-Indic digits to Latin digits.
// ProcessMaker forms frequently receive Persian digits from users typing on a
// Persian keyboard; numeric formatting must run on the normalised form.
func ToLatinDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 0x06F0 && r <= 0x06F9:
			b.WriteRune('0' + (r - 0x06F0))
		case r >= 0x0660 && r <= 0x0669:
			b.WriteRune('0' + (r - 0x0660))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizePersian folds the Arabic letter forms that Windows keyboards produce
// into their Persian equivalents: Arabic yeh (ي) to Persian yeh (ی), Arabic kaf
// (ك) to Persian kaf (ک), and Arabic-Indic digits to Persian ones.
//
// Without this, the same word typed on two machines sorts and renders
// differently, and some Persian fonts have no glyph for the Arabic forms.
func NormalizePersian(s string) string {
	r := strings.NewReplacer(
		"ي", "ی", // ي -> ی
		"ى", "ی", // ى -> ی
		"ك", "ک", // ك -> ک
		"ڪ", "ک", // ڪ -> ک
		"ة", "ه", // ة -> ه
	)
	return r.Replace(s)
}
