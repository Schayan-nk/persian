// Package translit writes a Latin company name the way Persian writes it.
//
// WHY THIS IS A MODEL'S JOB AND NOT A TABLE. Persian script does not write
// short vowels, so going Latin to Persian means putting back information the
// Latin spelling does not contain: 'Nikan' is «نیکان», but a letter-by-letter map
// has no way to know whether the vowel between the k and the a is written or
// not, and would produce «نیکان» and «نکان» with equal confidence. Worse, these
// are real company names, so the right answer is often not a transliteration
// at all -- 'Golzar PowerPlant' is «نیروگاه گلزار», where the proper noun is
// transliterated and 'PowerPlant' is TRANSLATED, because that is what the
// company is called. No rule table produces that.
//
// WHICH IS ALSO WHY EVERY ANSWER IS A GUESS UNTIL A PERSON CHECKS IT. A wrong
// Persian spelling reads as perfectly plausible -- there is no misspelling for
// a reader to notice, because both spellings are well-formed Persian. The
// store keeps the provenance for exactly this reason, and refuses to mark a
// machine's answer verified.
//
// The model call is injected rather than imported, so everything in here --
// the prompt, the alignment, the validation, the refusals -- is tested without
// a network and without a key.
package translit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Ask is the model call this package needs.
//
// Deliberately not aiadvice.Complete's own signature with a Config: this
// package has no business knowing what a provider is, and a test has no
// business constructing one.
type Ask func(ctx context.Context, system, prompt string) (string, error)

// BatchSize is how many names go in one request.
//
// Not larger, because alignment is the failure mode that matters: the reply is
// matched back to the input by line number, and a long list is where a model
// starts dropping or merging lines. Twenty is small enough that a drift is
// visible in one batch rather than silently spread across hundreds of rows,
// and large enough that the whole 922-row list is under fifty requests.
const BatchSize = 20

// maxGrowth bounds how much longer an answer may be than the name it answers.
//
// A model that starts explaining itself -- "This company's name in Persian
// would be ..." -- produces a line far longer than the name. Persian script
// is usually SHORTER than its Latin transliteration in characters, so a
// generous multiple still catches prose.
const maxGrowth = 3

// Result is one name and what the model made of it.
//
// There is always exactly one Result per input name, in the same order, even
// when the request failed. A caller storing these needs to know which names
// got nothing, and a shorter slice makes that somebody else's problem.
type Result struct {
	// Name is the Latin name as asked, before any cleaning for the prompt.
	Name string
	// Persian is the answer, or "" when there is none worth storing.
	Persian string
	// Why says what went wrong, and is empty when Persian is set. It exists
	// so a log can say "the model answered in Latin" rather than "0 of 20".
	Why string
}

// Ok reports whether this result carries a usable name.
func (r Result) Ok() bool { return r.Persian != "" }

// system is the instruction, and it is written in English on purpose.
//
// The advice prompts in aiadvice are Persian because their ANSWER is read in
// Persian beside Persian figures. Here the answer is a single name and the
// instruction is a specification -- what to translate, what to transliterate,
// what not to add -- which the models follow more reliably in English. What
// must be Persian is the output, and the instruction says so twice.
const system = `You transliterate Iranian company names from Latin script into Persian script.

Rules:
- Answer with Persian script only. No Latin letters anywhere in your answer.
- Transliterate proper nouns and place names (Golzar -> گلزار, Nikan -> نیکان).
- TRANSLATE generic business words rather than transliterating them:
  "Power Plant" -> نیروگاه, "Company" -> شرکت, "Refinery" -> پالایشگاه,
  "Warehouse" -> انبار, "Oil Products" -> فرآورده‌های نفتی, "Energy" -> انرژی,
  "Trading" -> بازرگانی, "Industries" -> صنایع, "Steel" -> فولاد.
- Keep an abbreviation that Iranians write in Latin as it is pronounced in
  Persian letters, not as Latin.
- Keep the word order Persian readers use for that kind of name.
- Do not add a word the Latin name does not contain. Do not explain.
- Do not add honorifics, legal forms, or a city that was not given.

Answer format: one line per input, "N. <name in Persian>", where N is the
number you were given. Nothing before the first line and nothing after the
last. If you genuinely cannot render one, write "N. -" for that line only.`

// Names transliterates every name, in batches of BatchSize.
//
// Always returns one Result per input name, in order. A batch whose request
// fails leaves its names with no Persian name and a Why saying what happened,
// because one unreachable model must not discard the batches that worked. An
// error is returned only when nothing could be attempted at all.
func Names(ctx context.Context, ask Ask, names []string) ([]Result, error) {
	if ask == nil {
		return nil, fmt.Errorf("translit: no model to ask")
	}
	out := make([]Result, len(names))
	for i, n := range names {
		out[i] = Result{Name: n}
	}
	if len(names) == 0 {
		return out, nil
	}

	for start := 0; start < len(names); start += BatchSize {
		end := min(start+BatchSize, len(names))
		batch := out[start:end]

		// Context cancellation stops the whole run rather than marking every
		// remaining batch as failed one request at a time.
		if err := ctx.Err(); err != nil {
			for i := range batch {
				if batch[i].Why == "" && batch[i].Persian == "" {
					batch[i].Why = "not attempted: " + err.Error()
				}
			}
			return out, nil
		}
		askBatch(ctx, ask, batch)
	}
	return out, nil
}

// askBatch fills in one batch in place.
func askBatch(ctx context.Context, ask Ask, batch []Result) {
	var b strings.Builder
	for i, r := range batch {
		fmt.Fprintf(&b, "%d. %s\n", i+1, forPrompt(r.Name))
	}

	reply, err := ask(ctx, system, b.String())
	if err != nil {
		for i := range batch {
			batch[i].Why = "the model did not answer: " + err.Error()
		}
		return
	}

	got := parse(reply, len(batch))
	for i := range batch {
		line, ok := got[i+1]
		if !ok {
			batch[i].Why = "the model's answer had no line for this name"
			continue
		}
		name, why := validate(batch[i].Name, line)
		batch[i].Persian, batch[i].Why = name, why
	}
}

// forPrompt makes one name safe to put on a numbered line.
//
// A newline inside a name would break the line numbering the reply is matched
// back by -- one name would arrive as two inputs and every answer after it
// would be attributed to the wrong company. These names come from this
// company's own database rather than from a stranger, so this is not about a
// hostile value; it is about a value with a line break in it silently
// mislabelling every row after it.
func forPrompt(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// parse pulls "N. text" lines out of a reply, keeping only numbers in range.
//
// Tolerant of what models actually do: a bullet before the number, ')' or ':'
// instead of '.', a blank line between entries, a stray sentence before the
// list. Intolerant of the one thing that matters -- a line whose number is not
// one of the numbers asked about is dropped rather than guessed at.
func parse(reply string, n int) map[int]string {
	out := make(map[int]string, n)
	for _, raw := range strings.Split(reply, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimLeft(line, "-*• \t")
		if line == "" {
			continue
		}
		// Leading digits, then a separator.
		i := 0
		for i < len(line) && line[i] >= '0' && line[i] <= '9' {
			i++
		}
		if i == 0 || i > 4 || i >= len(line) {
			continue
		}
		num, err := strconv.Atoi(line[:i])
		if err != nil || num < 1 || num > n {
			continue
		}
		rest := line[i:]
		// '-' is deliberately NOT in this set. It is the marker the
		// instruction asks for when the model cannot render a name, so
		// stripping it here would turn "2. -" into an empty line and report it
		// as "no answer for this name" instead of "the model declined".
		trimmed := strings.TrimLeft(rest, ".):، \t")
		if trimmed == rest {
			// Nothing at all between the number and the text: "12نیکان" is not a
			// numbered line.
			continue
		}
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		// A dash used as a bullet after the number -- "1. - نیکان" -- is
		// decoration. A dash that is the WHOLE answer is a refusal, and is
		// left alone for validate to report.
		if after, ok := strings.CutPrefix(trimmed, "-"); ok && strings.TrimSpace(after) != "" {
			trimmed = strings.TrimSpace(after)
		}
		// First answer for a number wins. A model that repeats itself usually
		// restates rather than corrects, and picking the later one would make
		// the result depend on how chatty the reply was.
		if _, seen := out[num]; !seen {
			out[num] = trimmed
		}
	}
	return out
}

// validate decides whether one answer is worth storing.
//
// Everything rejected here is something a model actually does: echoing the
// Latin input, leaving half the name untranslated, answering with a sentence,
// or refusing. Storing any of those would put a wrong name in front of a
// reader with no sign that it is wrong, which is worse than storing nothing --
// a blank is visibly a blank.
func validate(name, answer string) (persian, why string) {
	answer = strings.TrimSpace(answer)
	answer = strings.Trim(answer, "\"'«»")
	answer = strings.TrimSpace(answer)

	if answer == "" || answer == "-" || answer == "—" {
		return "", "the model declined to render this name"
	}
	var (
		arabic bool
		latin  bool
	)
	for _, r := range answer {
		switch {
		case unicode.Is(unicode.Arabic, r):
			arabic = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			latin = true
		}
	}
	if !arabic {
		return "", "the answer contains no Persian script: " + clip(answer)
	}
	if latin {
		// Half-translated is the common shape: «نیروگاه Golzar». It reads as
		// a mistake to anybody and must not be stored as the Persian name.
		return "", "the answer still contains Latin letters: " + clip(answer)
	}
	if len([]rune(answer)) > maxGrowth*len([]rune(name))+20 {
		return "", "the answer is prose rather than a name: " + clip(answer)
	}
	if strings.Contains(answer, "\n") {
		return "", "the answer is more than one line: " + clip(answer)
	}
	return answer, ""
}

// clip keeps a rejection message readable in a log.
func clip(s string) string {
	r := []rune(s)
	if len(r) <= 60 {
		return strconv.Quote(s)
	}
	return strconv.Quote(string(r[:60]) + "…")
}
