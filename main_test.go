package main

// These tests run the app against plugintest's fake filex: no wasm build,
// no server. The fake host takes the manifest's permissions as the
// administrator's grant, so a host call the manifest forgot to ask for is
// refused here exactly as filex refuses it, and a screen that tries to
// write is refused the way a real screen is.
//
//	go test ./...            everything below
//	go test ./... -update    rewrite testdata/golden after an intended change

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/brf-tech/filex/backend/pkg/pluginkit"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/plugintest"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

func harness() *plugintest.Harness {
	return plugintest.NewFor(Manifest(), func(h *plugintest.Host) *pluginkit.Plugin {
		return newPlugin(h)
	})
}

func txt(name, body string) plugintest.File {
	return plugintest.File{Name: name, Data: []byte(body), Mime: "text/plain"}
}

// ── the manifest ────────────────────────────────────────────────────────

// readManifestFile decodes filex-app.json the way filex does — an unknown
// key is refused — and answers it with and without its wasm block.
func readManifestFile(t *testing.T) (wire.Manifest, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile("filex-app.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m wire.Manifest
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("filex-app.json has a key filex would refuse: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	delete(generic, "wasm")
	return m, generic
}

// The module embeds filex-app.json minus its wasm block. Editing the
// manifest without regenerating the copy would ship a module that
// describes an older app.
func TestManifestEmbedIsFresh(t *testing.T) {
	_, disk := readManifestFile(t)
	var embedded map[string]any
	if err := json.Unmarshal(manifestJSON, &embedded); err != nil {
		t.Fatal(err)
	}
	if _, has := embedded["wasm"]; has {
		t.Fatal("manifest.embed.json must not carry the wasm block")
	}
	if !reflect.DeepEqual(disk, embedded) {
		t.Fatal("manifest.embed.json is stale: run `go generate ./...` (scripts/build.sh does it)")
	}
}

// What `describe` answers is the manifest the administrator approved:
// field for field, nothing lost on the way through the Go types.
func TestDescribeIsTheManifest(t *testing.T) {
	m, disk := readManifestFile(t)
	m.Wasm = nil
	want, _ := json.Marshal(m)
	got, _ := json.Marshal(harness().Manifest())
	if !bytes.Equal(want, got) {
		t.Fatalf("describe differs from filex-app.json\n got: %s\nwant: %s", got, want)
	}
	var described map[string]any
	if err := json.Unmarshal(got, &described); err != nil {
		t.Fatal(err)
	}
	if path, ok := contains(described, disk, "manifest"); !ok {
		t.Fatalf("filex-app.json says something describe does not: %s", path)
	}
}

// contains reports whether every key and value of want is in got (got may
// add empty defaults the Go types always write, such as "applies": {}).
func contains(got, want any, at string) (string, bool) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return at, false
		}
		for k, v := range w {
			if p, ok := contains(g[k], v, at+"."+k); !ok {
				return p, false
			}
		}
		return "", true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return at, false
		}
		for i := range w {
			if p, ok := contains(g[i], w[i], at); !ok {
				return p, false
			}
		}
		return "", true
	default:
		return at, reflect.DeepEqual(got, want)
	}
}

// What filex checks at install: the closed permission set, declared views,
// every Text in both declared languages, and a function for every id.
func TestManifestPassesFilexChecks(t *testing.T) {
	h := harness()
	plugintest.CheckManifest(t, h.Manifest())
	plugintest.CheckManifestLanguages(t, h.Manifest())
	plugintest.CheckRegistered(t, h.Plugin)
}

// ── the options screen ──────────────────────────────────────────────────

func TestOptionsScreen(t *testing.T) {
	h := harness()
	s, err := h.Open(viewOptions, txt("notes.txt", "hello world"))
	if err != nil {
		t.Fatal(err)
	}
	plugintest.CheckSurface(t, h.Manifest(), s)
	plugintest.CheckLanguages(t, h.Manifest(), s)
	plugintest.Golden(t, "options-one-file", s)

	if got := plugintest.ChoiceValues(s, "format"); !reflect.DeepEqual(got, []string{formatText, formatMarkdown}) {
		t.Fatalf("format choices = %v", got)
	}
	if _, ok := plugintest.Field(s, "combine"); ok {
		t.Fatal("one file: there is nothing to combine, so the switch is not asked")
	}
	if a, ok := plugintest.PrimaryAction(s); !ok || a.ID != buttonCount {
		t.Fatalf("primary button = %+v", a)
	}
}

func TestOptionsScreenForSeveralFiles(t *testing.T) {
	h := harness()
	s, err := h.Open(viewOptions, txt("a.txt", "one"), txt("b.md", "two"))
	if err != nil {
		t.Fatal(err)
	}
	plugintest.CheckSurface(t, h.Manifest(), s)
	if _, ok := plugintest.Field(s, "combine"); !ok {
		t.Fatal("several files: the screen asks whether to write one report")
	}
	plugintest.Golden(t, "options-two-files", s)
}

// The same screen in English and in Turkish, side by side: a label left in
// English under a Turkish heading is only visible this way.
func TestOptionsScreenInBothLanguages(t *testing.T) {
	h := harness()
	byLocale, err := h.OpenInLocales(viewOptions, txt("a.txt", "one"), txt("b.md", "two"))
	if err != nil {
		t.Fatal(err)
	}
	plugintest.CheckLocaleParityOpts(t, byLocale, plugintest.LangOpts{Strict: true})
}

func TestOptionsSubmitQueuesTheJob(t *testing.T) {
	h := harness()
	h.Select(txt("a.txt", "the cat and the hat"), txt("b.txt", "the end"))
	s, err := h.Submit(viewOptions, nil, map[string]any{"format": "md", "top": float64(2), "combine": true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Job == nil || s.Job.ActionID != actionCount {
		t.Fatalf("submit should queue %q, got %+v", actionCount, s)
	}
	out, err := h.Queue(s)
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || len(out.Outputs) != 1 || out.Outputs[0].Name != "wordcount-report.md" {
		t.Fatalf("one combined report expected, got %+v", out)
	}
	body, _ := h.Host.Bytes(out.Outputs[0].Ref)
	for _, want := range []string{"## a.txt", "## b.txt", "## Total — 2 files", "| Words | 7 |", "| 1 | the | 2 |"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("report lacks %q:\n%s", want, body)
		}
	}
}

func TestOptionsRefuseAnOutOfRangeNumber(t *testing.T) {
	h := harness()
	h.Select(txt("a.txt", "x"))
	s, err := h.Submit(viewOptions, nil, map[string]any{"format": "txt", "top": float64(99)})
	if err != nil {
		t.Fatal(err)
	}
	if s.Job != nil {
		t.Fatal("an invalid form must not queue a job")
	}
	if e := s.Errors["top"]; e["en"] == "" || e["tr"] == "" {
		t.Fatalf("the error on top must be in both languages, got %v", s.Errors)
	}
	if got := plugintest.ValuesOf(plugintest.Forms(s)[0])["top"]; got != float64(99) {
		t.Fatalf("the refused value is shown again, not reset; got %v", got)
	}
}

// ── the job ─────────────────────────────────────────────────────────────

func TestCountWritesOneReportPerFile(t *testing.T) {
	h := harness()
	out, err := h.Do(actionCount, map[string]any{"format": "txt", "top": float64(1)},
		txt("a.txt", "red red blue\n"), txt("b.txt", "green"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range out.Outputs {
		names = append(names, o.Name)
	}
	if !reflect.DeepEqual(names, []string{"a-wordcount.txt", "b-wordcount.txt"}) {
		t.Fatalf("outputs = %v", names)
	}
	a, _ := h.Host.Bytes(out.Outputs[0].Ref)
	for _, want := range []string{"Words:       3", "Lines:       1", "1. red", "2\n"} {
		if !strings.Contains(string(a), want) {
			t.Errorf("a-wordcount.txt lacks %q:\n%s", want, a)
		}
	}
	if out.Message["en"] != "Counted 4 words in 2 files." || out.Message["tr"] != "2 dosyada 4 kelime sayıldı." {
		t.Fatalf("message = %v", out.Message)
	}
	if n := len(h.Host.ProgressLog); n == 0 {
		t.Fatal("the job reports progress")
	}
}

// The report is written in the language of the person who ran it.
func TestCountSpeaksTurkish(t *testing.T) {
	h := harness()
	h.Locale = "tr"
	body := strings.Repeat("kelime ", 1234) + "ışık IŞIK ışık"
	out, err := h.Do(actionCount, map[string]any{"format": "txt", "top": float64(2)}, txt("metin.txt", body))
	if err != nil {
		t.Fatal(err)
	}
	report, _ := h.Host.Bytes(out.Outputs[0].Ref)
	for _, want := range []string{"Kelime:      1.237", "En sık geçen kelimeler", "1. kelime", "1.234", "2. ışık"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("Turkish report lacks %q:\n%s", want, report)
		}
	}
}

func TestCountSkipsWhatIsNotText(t *testing.T) {
	h := harness()
	out, err := h.Do(actionCount, nil, txt("a.txt", "fine"), plugintest.File{Name: "b.txt", Data: []byte{0xff, 0xfe, 0x00}})
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK || len(out.Outputs) != 1 || !strings.Contains(out.Message["en"], "Skipped: b.txt") {
		t.Fatalf("got %+v", out)
	}
	out, err = h.Do(actionCount, nil, plugintest.File{Name: "c.txt", Data: []byte{0xff}})
	if err != nil {
		t.Fatal(err)
	}
	if out.OK {
		t.Fatal("nothing counted is a failed job, with the reason")
	}
}

// The job checks its params itself: the API runs an action with any
// params, not only the ones the screen allows.
func TestCountRefusesBadParams(t *testing.T) {
	h := harness()
	out, err := h.Do(actionCount, map[string]any{"format": "pdf"}, txt("a.txt", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if out.OK || out.Message["en"] == "" {
		t.Fatalf("got %+v", out)
	}
}

// ── the details panel ───────────────────────────────────────────────────

func TestStatsPanel(t *testing.T) {
	h := harness()
	s, err := h.Open(viewStats, txt("notes.txt", "one two two\nthree"))
	if err != nil {
		t.Fatal(err)
	}
	plugintest.CheckSurface(t, h.Manifest(), s)
	plugintest.CheckLanguages(t, h.Manifest(), s)
	first := s.Nodes[0].Props["text"].(wire.Text)
	if first["en"] != "4 words · 2 lines · 17 characters" || first["tr"] != "4 kelime · 2 satır · 17 karakter" {
		t.Fatalf("stats line = %v", first)
	}
	plugintest.Golden(t, "stats", s)
}

// "Write a report…" in the panel starts the menu's action on this file.
func TestStatsPanelOpensTheAction(t *testing.T) {
	h := harness()
	h.Select(plugintest.File{Name: "notes.txt", Data: []byte("a b"), Path: "docs://notes.txt"})
	s, err := h.Act(viewStats, buttonReport, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Open == nil || s.Open.Path != "docs://notes.txt" || s.Open.Action != actionCount {
		t.Fatalf("open = %+v", s.Open)
	}
}

func TestStatsPanelLeavesLargeFilesToTheJob(t *testing.T) {
	h := harness()
	s, err := h.Open(viewStats, txt("big.log", strings.Repeat("x ", maxScreenBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Nodes[0].Props["text"].(wire.Text)["en"]; !strings.Contains(got, "too large") {
		t.Fatalf("got %q", got)
	}
}

// ── the counting itself ─────────────────────────────────────────────────

func TestCount(t *testing.T) {
	for _, c := range []struct {
		text                string
		words, lines, chars int
	}{
		{"", 0, 0, 0},
		{"one", 1, 1, 3},
		{"one\n", 1, 1, 4},
		{"one\ntwo\n\n", 2, 3, 9},
		{"don't e-mail Ankara'da", 3, 1, 22},
		{"a -- b", 2, 1, 6},
		{"\xef\xbb\xbfbom", 1, 1, 3},
		{"çok güzel, değil mi?", 4, 1, 20},
	} {
		s, err := Count("x", []byte(c.text), 0)
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		if s.Words != c.words || s.Lines != c.lines || s.Chars != c.chars {
			t.Errorf("%q: words %d lines %d chars %d, want %d %d %d", c.text, s.Words, s.Lines, s.Chars, c.words, c.lines, c.chars)
		}
	}
}

func TestTopWordsFoldTurkishAndEnglishCase(t *testing.T) {
	s, _ := Count("x", []byte("ışık IŞIK ışık Işık invoice INVOICE invoice"), 5)
	want := []WordCount{{"ışık", 4}, {"invoice", 3}}
	if !reflect.DeepEqual(s.Top, want) {
		t.Fatalf("top = %v, want %v", s.Top, want)
	}
}

func TestNumber(t *testing.T) {
	for _, c := range []struct {
		lang string
		n    int
		want string
	}{{"en", 0, "0"}, {"en", 999, "999"}, {"en", 1234567, "1,234,567"}, {"tr", 1234, "1.234"}, {"tr-TR", -12345, "-12.345"}} {
		if got := number(c.lang, c.n); got != c.want {
			t.Errorf("number(%s, %d) = %s, want %s", c.lang, c.n, got, c.want)
		}
	}
}
