package main

// This file is the app's own work — no filex in it. Replace it with yours.

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Stats is what one text says about itself.
type Stats struct {
	Name  string
	Words int
	Lines int
	Chars int // Unicode characters, not bytes
	Top   []WordCount
}

// WordCount is one entry of the most-frequent list.
type WordCount struct {
	Word  string
	Count int
}

// errNotText is returned for a file that is not UTF-8 text.
var errNotText = errors.New("not UTF-8 text")

// Count reads a text and keeps the `top` most frequent words.
//
// A word is a run of letters, digits and combining marks; an apostrophe or
// a hyphen BETWEEN two letters stays inside it, so "don't", "e-mail" and
// "Ankara'da" are one word each.
func Count(name string, data []byte, top int) (Stats, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // a UTF-8 byte order mark is not text
	if !utf8.Valid(data) {
		return Stats{Name: name}, errNotText
	}
	s := Stats{Name: name, Chars: utf8.RuneCount(data)}
	if len(data) > 0 {
		s.Lines = bytes.Count(data, []byte("\n"))
		if data[len(data)-1] != '\n' {
			s.Lines++
		}
	}
	freq := map[string]*tally{}
	text := []rune(string(data))
	var word []rune
	flush := func() {
		if len(word) == 0 {
			return
		}
		s.Words++
		w := string(word)
		k := foldWord(w)
		if freq[k] == nil {
			freq[k] = &tally{spellings: map[string]int{}, first: w}
		}
		freq[k].count++
		freq[k].spellings[w]++
		word = word[:0]
	}
	for i, r := range text {
		switch {
		case isWordRune(r):
			word = append(word, r)
		case (r == '\'' || r == '’' || r == '-') && len(word) > 0 && i+1 < len(text) && unicode.IsLetter(text[i+1]):
			word = append(word, r)
		default:
			flush()
		}
	}
	flush()
	s.Top = topWords(freq, top)
	return s, nil
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

// foldWord is the key the spellings of one word share: "The" and "the"
// are one word. It is a MATCHING rule, never shown. strings.ToLower alone
// folds "I" to "i", which is wrong for Turkish ("IŞIK" is "ışık"), while
// the Turkish rule folds "INVOICE" to "ınvoıce"; treating I, İ, ı and i as
// one letter counts both languages right.
func foldWord(w string) string {
	return strings.ToLower(strings.NewReplacer("I", "i", "İ", "i", "ı", "i").Replace(w))
}

// tally is one word: how often it occurs, and how it is written.
type tally struct {
	count     int
	spellings map[string]int
	first     string
}

// shown is the word as the text writes it most often (the first spelling
// on a tie) — a report says "ışık", not the folded key "işik".
func (w *tally) shown() string {
	best, n := w.first, w.spellings[w.first]
	for s, c := range w.spellings {
		if c > n {
			best, n = s, c
		}
	}
	return best
}

func topWords(freq map[string]*tally, n int) []WordCount {
	if n <= 0 {
		return nil
	}
	all := make([]WordCount, 0, len(freq))
	for _, w := range freq {
		all = append(all, WordCount{w.shown(), w.count})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Word < all[j].Word
	})
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// Report formats.
const (
	formatText     = "txt"
	formatMarkdown = "md"
)

// Report writes the report for one or more files in the reader's language
// (`lang`, the job's locale) and format. More than one file adds a total.
func Report(lang, format string, files []Stats) []byte {
	var b strings.Builder
	md := format == formatMarkdown
	for i, s := range files {
		if i > 0 {
			b.WriteString("\n")
		}
		writeOne(&b, lang, md, s)
	}
	if len(files) > 1 {
		var total Stats
		for _, s := range files {
			total.Words += s.Words
			total.Lines += s.Lines
			total.Chars += s.Chars
		}
		b.WriteString("\n")
		title := fmt.Sprintf(t("Total — %d files", "Toplam — %d dosya").Get(lang), len(files))
		writeCounts(&b, lang, md, title, total)
	}
	return []byte(b.String())
}

func writeOne(b *strings.Builder, lang string, md bool, s Stats) {
	writeCounts(b, lang, md, s.Name, s)
	if len(s.Top) == 0 {
		return
	}
	heading := t("Most frequent words", "En sık geçen kelimeler").Get(lang)
	if md {
		fmt.Fprintf(b, "\n### %s\n\n| # | %s | %s |\n|---:|---|---:|\n", heading,
			t("Word", "Kelime").Get(lang), t("Count", "Adet").Get(lang))
		for i, w := range s.Top {
			fmt.Fprintf(b, "| %d | %s | %s |\n", i+1, w.Word, number(lang, w.Count))
		}
		return
	}
	fmt.Fprintf(b, "\n%s\n", heading)
	for i, w := range s.Top {
		fmt.Fprintf(b, "%4d. %-24s %s\n", i+1, w.Word, number(lang, w.Count))
	}
}

func writeCounts(b *strings.Builder, lang string, md bool, title string, s Stats) {
	rows := [][2]string{
		{t("Words", "Kelime").Get(lang), number(lang, s.Words)},
		{t("Lines", "Satır").Get(lang), number(lang, s.Lines)},
		{t("Characters", "Karakter").Get(lang), number(lang, s.Chars)},
	}
	if md {
		fmt.Fprintf(b, "## %s\n\n| | |\n|---|---:|\n", title)
		for _, r := range rows {
			fmt.Fprintf(b, "| %s | %s |\n", r[0], r[1])
		}
		return
	}
	fmt.Fprintf(b, "%s\n%s\n", title, strings.Repeat("=", utf8.RuneCountInString(title)))
	for _, r := range rows {
		fmt.Fprintf(b, "%-12s %s\n", r[0]+":", r[1])
	}
}

// number groups the digits the way the reader writes them: 12,345 in
// English, 12.345 in Turkish. Add your languages here.
func number(lang string, n int) string {
	sep := ","
	if baseLang(lang) == "tr" {
		sep = "."
	}
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	if neg {
		return "-" + strings.Join(out, sep)
	}
	return strings.Join(out, sep)
}

func baseLang(lang string) string {
	lang, _, _ = strings.Cut(strings.ToLower(lang), "-")
	return lang
}
