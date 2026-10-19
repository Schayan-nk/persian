# persian

Go packages for handling Persian text in business software: comparing
spellings, keeping mixed Persian/English text in the right order, reading
Finglish, the Jalali calendar, numbers in words, and Persian spellings of
Latin company names.

| Package | What it does |
| --- | --- |
| [`fa`](fa) | Folds Persian text so two spellings of one word compare equal: Arabic and Persian letters, kashida, diacritics and the digit sets. |
| [`bidi`](bidi) | Classifies text direction, splits mixed Persian/English strings into direction-homogeneous segments, converts digits, wraps values in Unicode isolates. |
| [`finglish`](finglish) | Reads Persian typed in Latin letters ("vaziate sefaresh chie?") as the Persian it is, for the few hundred words people actually type, and refuses to fire on English or German. |
| [`format`](format) | Jalali calendar, numbers to Persian or English words, and a small filter pipeline for turning values into document text (`number:2\|fa`, `jdate`, `money`, `fawords`...). |
| [`translit`](translit) | Writes Latin company names the way Persian writes them, through a language model you inject, with the prompt, alignment and validation that make its answers usable. |

No dependencies outside the standard library.

## Install

```sh
go get github.com/Schayan-nk/persian@latest
```

## Examples

```go
import (
	"github.com/Schayan-nk/persian/bidi"
	"github.com/Schayan-nk/persian/fa"
	"github.com/Schayan-nk/persian/finglish"
	"github.com/Schayan-nk/persian/format"
)

fa.Fold("كتابي") == fa.Fold("کتابی")         // true: Arabic kaf and yeh fold to Persian

bidi.ToPersianDigits("Order 2026-10")         // "Order ۲۰۲۶-۱۰"
bidi.ToLatinDigits("۱۴۰۵/۰۷/۲۶")              // "1405/07/26"
bidi.Split("شرکت نمونه Example Co. ۱۲۳", bidi.RTL)
// two segments: "شرکت نمونه " (RTL) and "Example Co. ۱۲۳" (LTR)

finglish.Is("vaziate sefaresh 204518 chie?")  // true
finglish.ToPersian("vaziate sefaresh 204518 chie?") // "وضعیت سفارش 204518 چیه?"
finglish.Is("where is my order?")             // false

format.ToJalali(2026, 10, 18)                 // 1405, 7, 26
format.NumberToPersianWords("1250")           // "یک هزار و دویست و پنجاه"
format.Value(1234567.891, format.ParseFilters("number:2|fa"), format.DefaultOptions())
// "۱,۲۳۴,۵۶۷.۸۹"
format.Value("2026-10-18", format.ParseFilters("jdate|fa"), format.DefaultOptions())
// "۱۴۰۵/۰۷/۲۶"
```

`translit` takes the model call as a function, so it works with any provider
and is tested without a network:

```go
ask := func(ctx context.Context, system, prompt string) (string, error) {
	// call your language model here
}
results, err := translit.Names(ctx, ask, []string{"Nikan PowerPlant"})
```

Every answer is a guess until a person checks it: a wrong Persian spelling of
a name reads as perfectly well-formed Persian, so there is nothing for a
reader to notice. Keep the provenance and have someone confirm.

## Why bidi matters for documents

In Word documents (WordprocessingML) the run is the unit that carries
direction. If one run holds both «شرکت نمونه» and "Example Co.", Word has to
guess, and the result is the mangled ordering people see when a value is
dropped into a template verbatim. `bidi.Split` gives one segment per
direction, so each can be written as its own run.

## Tests

```sh
go test ./...
```

## License

MIT, see [LICENSE](LICENSE).
