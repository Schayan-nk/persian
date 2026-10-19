package translit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The parsing and the refusals, without a network.
//
// Every rejected shape below is something a model actually produces: echoing
// the Latin input, translating half the name, answering with a sentence,
// dropping a line from the middle of a list, renumbering. The point of the
// package is that none of those reach the database, because a wrong Persian
// name reads as perfectly plausible -- there is no misspelling for a reader to
// notice -- while a blank is visibly a blank.

// replyFrom builds a well-formed answer from the lines given.
//
// Iterates to the highest KEY rather than to len(lines), so a map with a gap
// in it -- which is how the dropped-line test states its case -- still emits
// the lines after the gap.
func replyFrom(lines map[int]string) Ask {
	high := 0
	for n := range lines {
		if n > high {
			high = n
		}
	}
	return func(ctx context.Context, system, prompt string) (string, error) {
		var b strings.Builder
		for i := 1; i <= high; i++ {
			if s, ok := lines[i]; ok {
				fmt.Fprintf(&b, "%d. %s\n", i, s)
			}
		}
		return b.String(), nil
	}
}

func TestAWellFormedAnswerLandsOnTheRightNames(t *testing.T) {
	got, err := Names(context.Background(), replyFrom(map[int]string{
		1: "نیکان",
		2: "نیروگاه گلزار",
		3: "انبار ای بی سی",
	}), []string{"Nikan", "Golzar PowerPlant", "Abc Stock Warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"نیکان", "نیروگاه گلزار", "انبار ای بی سی"}
	if len(got) != 3 {
		t.Fatalf("got %d results for 3 names", len(got))
	}
	for i := range want {
		if got[i].Persian != want[i] {
			t.Errorf("%q -> %q, want %q (why: %s)",
				got[i].Name, got[i].Persian, want[i], got[i].Why)
		}
		if !got[i].Ok() {
			t.Errorf("%q is not Ok", got[i].Name)
		}
	}
}

// AN ANSWER WITH LATIN IN IT IS THE COMMON FAILURE: the generic half is
// translated and the proper noun is left alone, giving «نیروگاه Golzar».
// It reads as a mistake to anybody and must never be stored.
func TestAnAnswerContainingLatinIsRejected(t *testing.T) {
	for _, bad := range []string{
		"نیروگاه Golzar",
		"Golzar نیروگاه",
		"Nikan",
		"نیکان (Nikan)",
	} {
		got, err := Names(context.Background(),
			replyFrom(map[int]string{1: bad}), []string{"Golzar PowerPlant"})
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Ok() {
			t.Errorf("%q was accepted", bad)
		}
		if !strings.Contains(got[0].Why, "Latin") && !strings.Contains(got[0].Why, "Persian script") {
			t.Errorf("%q was rejected for the wrong reason: %s", bad, got[0].Why)
		}
	}
}

// An answer with no Persian at all -- a refusal in English, a number, an empty
// apology -- is not a name.
func TestAnAnswerWithNoPersianIsRejected(t *testing.T) {
	for _, bad := range []string{
		"I cannot translate this",
		"???",
		"1023",
		"N/A",
	} {
		got, _ := Names(context.Background(),
			replyFrom(map[int]string{1: bad}), []string{"Nikan"})
		if got[0].Ok() {
			t.Errorf("%q was accepted as a Persian name", bad)
		}
	}
}

// A model that starts explaining produces a line far longer than the name.
func TestProseIsRejected(t *testing.T) {
	long := "نام این شرکت در فارسی به این صورت نوشته می‌شود و باید توجه داشت که " +
		"این نام از دو بخش تشکیل شده است که هر یک معنای خاصی دارد و در متون رسمی"
	got, _ := Names(context.Background(),
		replyFrom(map[int]string{1: long}), []string{"Nikan"})
	if got[0].Ok() {
		t.Error("a paragraph was accepted as a company name")
	}
	if !strings.Contains(got[0].Why, "prose") {
		t.Errorf("rejected for the wrong reason: %s", got[0].Why)
	}
}

// An explicit refusal for one line is recorded as such, and does not affect
// the others.
func TestADeclinedLineIsRecordedAndDoesNotAffectItsNeighbours(t *testing.T) {
	got, _ := Names(context.Background(), replyFrom(map[int]string{
		1: "نیکان",
		2: "-",
		3: "روناک انرژی",
	}), []string{"Nikan", "Xyzzy Qwerty", "Ronak Energy"})

	if !got[0].Ok() || !got[2].Ok() {
		t.Errorf("a refusal in the middle lost its neighbours: %+v", got)
	}
	if got[1].Ok() {
		t.Error("a dash was stored as a name")
	}
	if !strings.Contains(got[1].Why, "declined") {
		t.Errorf("why is %q", got[1].Why)
	}
}

// A DROPPED LINE MUST NOT SHIFT THE REST. This is the failure that would be
// invisible and catastrophic: if line 3 went missing and the answers after it
// slid up by one, every company from there on would get another company's
// name, all of them well-formed Persian and all of them wrong.
func TestADroppedLineDoesNotShiftTheOthers(t *testing.T) {
	got, _ := Names(context.Background(), replyFrom(map[int]string{
		1: "الف",
		2: "ب",
		// 3 is missing on purpose.
		4: "ت",
	}), []string{"Alef", "Bee", "Pay", "Tea"})

	if got[0].Persian != "الف" || got[1].Persian != "ب" {
		t.Errorf("the first two moved: %+v", got[:2])
	}
	if got[2].Ok() {
		t.Errorf("the missing line was filled from somewhere: %q", got[2].Persian)
	}
	if !strings.Contains(got[2].Why, "no line") {
		t.Errorf("why is %q", got[2].Why)
	}
	if got[3].Persian != "ت" {
		t.Errorf("the line after the gap is %q, want ت -- the answers slid", got[3].Persian)
	}
}

// A number the request never asked about is dropped rather than guessed at.
func TestALineNumberedOutOfRangeIsIgnored(t *testing.T) {
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		return "0. صفر\n1. یک\n7. هفت\n99. نود\n", nil
	}
	got, _ := Names(context.Background(), ask, []string{"One", "Two"})
	if got[0].Persian != "یک" {
		t.Errorf("1 -> %q", got[0].Persian)
	}
	if got[1].Ok() {
		t.Errorf("2 was filled from an out-of-range line: %q", got[1].Persian)
	}
}

// Models decorate lists. None of that should matter.
func TestTheParserToleratesHowModelsActuallyFormatLists(t *testing.T) {
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		return "Here are the names:\n\n" +
			"- 1) نیکان\n" +
			"* 2: نیروگاه گلزار\n" +
			"  3.   روناک انرژی  \n" +
			"\nLet me know if you need more.\n", nil
	}
	got, _ := Names(context.Background(), ask,
		[]string{"Nikan", "Golzar PowerPlant", "Ronak Energy"})
	for i, want := range []string{"نیکان", "نیروگاه گلزار", "روناک انرژی"} {
		if got[i].Persian != want {
			t.Errorf("%d -> %q, want %q (why %s)", i+1, got[i].Persian, want, got[i].Why)
		}
	}
}

// A number and a space, with no punctuation, IS a numbered line -- models
// write lists that way and refusing them would empty a whole run.
//
// The hazard that justifies being fussy about line numbers is a sentence that
// happens to start with a digit. That is caught a step later by validation
// rather than by the parser, which is the better place for it: the parser
// cannot tell a name from a sentence, and validation can.
func TestASpaceIsEnoughOfASeparatorAndProseIsStillCaught(t *testing.T) {
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		return "1 نیکان\n", nil
	}
	got, _ := Names(context.Background(), ask, []string{"Nikan"})
	if got[0].Persian != "نیکان" {
		t.Errorf("a space-separated line was not read: %q (why %s)", got[0].Persian, got[0].Why)
	}

	// And an English sentence beginning with a digit reaches validation, which
	// rejects it for having no Persian in it.
	ask = func(ctx context.Context, system, prompt string) (string, error) {
		return "1 companies are listed below\n", nil
	}
	got, _ = Names(context.Background(), ask, []string{"Nikan"})
	if got[0].Ok() {
		t.Errorf("prose was stored as a name: %q", got[0].Persian)
	}
}

// A number with nothing between it and the text is not a numbered line.
func TestADigitGluedToTheTextIsNotALineNumber(t *testing.T) {
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		return "1نیکان\n", nil
	}
	got, _ := Names(context.Background(), ask, []string{"Nikan"})
	if got[0].Ok() {
		t.Errorf("a glued line was parsed as numbered: %q", got[0].Persian)
	}
}

// Quotes and guillemets are stripped; the name inside them is kept.
func TestSurroundingQuotesAreStripped(t *testing.T) {
	for _, in := range []string{`"نیکان"`, `'نیکان'`, `«نیکان»`} {
		got, _ := Names(context.Background(),
			replyFrom(map[int]string{1: in}), []string{"Nikan"})
		if got[0].Persian != "نیکان" {
			t.Errorf("%q -> %q", in, got[0].Persian)
		}
	}
}

// A repeated number takes the first answer, because a model that says it twice
// is usually restating rather than correcting -- and picking the later one
// would make the result depend on how chatty the reply was.
func TestARepeatedNumberTakesTheFirstAnswer(t *testing.T) {
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		return "1. نیکان\n1. نکان\n", nil
	}
	got, _ := Names(context.Background(), ask, []string{"Nikan"})
	if got[0].Persian != "نیکان" {
		t.Errorf("got %q, want the first answer نیکان", got[0].Persian)
	}
}

// A NAME WITH A NEWLINE IN IT CANNOT BREAK THE NUMBERING. One name arriving as
// two prompt lines would make every answer after it belong to the wrong
// company.
func TestANameContainingANewlineCannotBreakTheAlignment(t *testing.T) {
	var sawLines int
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		sawLines = len(strings.Split(strings.TrimSpace(prompt), "\n"))
		return "1. الف ب\n2. روناک انرژی\n", nil
	}
	got, _ := Names(context.Background(), ask,
		[]string{"Alef\nBee", "Ronak Energy"})
	if sawLines != 2 {
		t.Errorf("the prompt had %d lines for 2 names; a name's newline leaked in", sawLines)
	}
	if got[1].Persian != "روناک انرژی" {
		t.Errorf("the second name got %q", got[1].Persian)
	}
}

// Every name comes back, in order, however many batches it took.
func TestEveryNameComesBackInOrderAcrossBatches(t *testing.T) {
	const n = BatchSize*2 + 3
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("Company %d", i)
	}
	calls := 0
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		calls++
		// Answer each line with a Persian word plus the index, so a shifted
		// result is detectable.
		var b strings.Builder
		for i, line := range strings.Split(strings.TrimSpace(prompt), "\n") {
			// Recover the company number from the prompt line itself.
			num := strings.TrimSpace(strings.SplitN(line, ".", 2)[1])
			fmt.Fprintf(&b, "%d. شرکت %s\n", i+1, strings.TrimPrefix(num, "Company "))
		}
		return b.String(), nil
	}

	got, err := Names(context.Background(), ask, names)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("got %d results for %d names", len(got), n)
	}
	if calls != 3 {
		t.Errorf("%d requests for %d names at batch size %d; want 3", calls, n, BatchSize)
	}
	for i := range got {
		want := fmt.Sprintf("شرکت %d", i)
		if got[i].Persian != want {
			t.Errorf("%q -> %q, want %q -- results are misaligned across batches",
				got[i].Name, got[i].Persian, want)
		}
	}
}

// A MODEL THAT IS DOWN MUST NOT DISCARD THE BATCHES THAT WORKED. Every name
// still comes back, and the ones that failed say so.
func TestAFailedRequestLeavesAReasonRatherThanAnError(t *testing.T) {
	names := make([]string, BatchSize+2)
	for i := range names {
		names[i] = fmt.Sprintf("Company %d", i)
	}
	calls := 0
	ask := func(ctx context.Context, system, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("connection refused")
		}
		return "1. الف\n2. ب\n", nil
	}

	got, err := Names(context.Background(), ask, names)
	if err != nil {
		t.Fatalf("a failed batch became a fatal error: %v", err)
	}
	if len(got) != len(names) {
		t.Fatalf("got %d results for %d names", len(got), len(names))
	}
	for i := 0; i < BatchSize; i++ {
		if got[i].Ok() {
			t.Errorf("%d was filled despite the request failing", i)
		}
		if !strings.Contains(got[i].Why, "connection refused") {
			t.Errorf("%d: why is %q, which does not say what happened", i, got[i].Why)
		}
	}
	// The second batch still worked.
	if got[BatchSize].Persian != "الف" || got[BatchSize+1].Persian != "ب" {
		t.Errorf("the surviving batch was lost: %+v", got[BatchSize:])
	}
}

// A cancelled context stops asking rather than failing every remaining batch
// one request at a time.
func TestACancelledContextStopsAsking(t *testing.T) {
	names := make([]string, BatchSize*3)
	for i := range names {
		names[i] = fmt.Sprintf("Company %d", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	ask := func(c context.Context, system, prompt string) (string, error) {
		calls++
		cancel() // cancelled part way through the first batch
		return "1. الف\n", nil
	}
	got, err := Names(ctx, ask, names)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("made %d requests after cancellation; want 1", calls)
	}
	if len(got) != len(names) {
		t.Errorf("got %d results for %d names", len(got), len(names))
	}
	if !strings.Contains(got[BatchSize].Why, "not attempted") {
		t.Errorf("a name in a skipped batch says %q", got[BatchSize].Why)
	}
}

// No names is not an error.
func TestNoNamesIsNotAnError(t *testing.T) {
	got, err := Names(context.Background(), replyFrom(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results for no names", len(got))
	}
}

// No model at all is an error, because there is nothing to report per name.
func TestNoModelIsAnError(t *testing.T) {
	if _, err := Names(context.Background(), nil, []string{"Nikan"}); err == nil {
		t.Error("a nil Ask was accepted")
	}
}

// The instruction has to say the things the validation enforces, or the
// validation is rejecting answers nobody asked it not to give.
func TestTheInstructionAsksForWhatTheValidationRequires(t *testing.T) {
	for _, want := range []string{"Persian script only", "No Latin letters", "TRANSLATE"} {
		if !strings.Contains(system, want) {
			t.Errorf("the system message does not mention %q", want)
		}
	}
}
