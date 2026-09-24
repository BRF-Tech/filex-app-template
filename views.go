package main

// The screens. A screen is data — a tree of nodes filex draws with its own
// components (text, form, list, steps, …; the catalogue is in
// docs/PLUGIN-KIT.md → "Screens"). filex calls the view again on every
// event (open, change, submit, action) with what the person did, and the
// view answers the next screen. Nothing survives between calls except what
// the event carries back.
//
// A screen may READ the selected files (files:read) but never write:
// writing is a job's business, which is why the options screen answers
// with a Job instead of counting itself.

import (
	"fmt"
	"strings"

	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

// Footer button ids. The primary button posts `submit`; every other
// button posts `action` with its id. There is no Cancel: filex puts a
// Close button beside yours on every dialog.
const (
	buttonCount  = "count"
	buttonReport = "report"
)

// options is the dialog "Word count…" opens: what kind of report, how many
// frequent words, and — for several files — one report or one per file.
func (a *app) options(in *wire.ViewEventInput) (*wire.Surface, error) {
	values := formValues(in)
	p, errs := readParams(values)
	if in.Event != "submit" {
		errs = nil // say what is wrong when they press Count, not while they type
	} else if len(errs) == 0 {
		if len(in.Context.Inputs) < 2 {
			p.Combine = false
		}
		// Ask filex to queue the action with these values. It checks the
		// person may run it on these files, then runs `count` as a job.
		return &wire.Surface{Job: &wire.JobRequest{ActionID: actionCount, Params: p.toMap()}}, nil
	}
	return optionsScreen(in.Context, values, errs), nil
}

// formValues is the form as the person left it, over the defaults — so an
// answer that failed a check is shown again rather than reset.
func formValues(in *wire.ViewEventInput) map[string]any {
	out := defaultParams().toMap()
	if v, ok := in.Data["values"].(map[string]any); ok {
		for k, val := range v {
			out[k] = val
		}
	}
	return out
}

func optionsScreen(ctx wire.CallContext, values map[string]any, errs map[string]wire.Text) *wire.Surface {
	lang := ctx.Locale
	// A form field's label is ONE string, in the call's language (unlike a
	// Text, which carries every language and is resolved by the browser).
	fields := []wire.Field{
		{Key: "format", Type: "select", Required: true,
			Label: t("Report format", "Rapor biçimi").Get(lang),
			Options: []wire.FieldOption{
				{Value: formatText, Label: t("Plain text (.txt)", "Düz metin (.txt)").Get(lang)},
				{Value: formatMarkdown, Label: t("Markdown table (.md)", "Markdown tablosu (.md)").Get(lang)},
			}},
		{Key: "top", Type: "int", Min: intPtr(0), Max: intPtr(maxTop),
			Label: t("Most frequent words to list", "Listelenecek en sık kelime sayısı").Get(lang),
			Help:  t("0 lists none.", "0 yazarsanız liste eklenmez.").Get(lang)},
	}
	if len(ctx.Inputs) > 1 {
		fields = append(fields, wire.Field{Key: "combine", Type: "bool", Style: "switch",
			Label: t("One report for all files", "Tüm dosyalar için tek rapor").Get(lang),
			Help:  t("Off: a report beside each file.", "Kapalıysa her dosyanın yanına ayrı bir rapor yazılır.").Get(lang)})
	} else {
		delete(values, "combine")
	}
	return &wire.Surface{
		Title: t("Word count", "Kelime sayımı"),
		Size:  "md",
		Nodes: []wire.Node{
			{Type: "text", Props: map[string]any{"text": selectionLine(ctx.Inputs)}},
			{ID: "options", Type: "form", Props: map[string]any{"fields": fields, "values": values}},
		},
		Actions: []wire.SurfaceAction{
			{ID: buttonCount, Label: t("Count", "Say"), Primary: true},
		},
		Errors: errs,
	}
}

// selectionLine says which files the report is about.
func selectionLine(files []wire.FileRef) wire.Text {
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}
	list := strings.Join(names, ", ")
	if len(files) == 1 {
		return t("Counts the words, lines and characters of "+list+".",
			list+" dosyasındaki kelime, satır ve karakterleri sayar.")
	}
	return t(fmt.Sprintf("Counts the words, lines and characters of %d files: %s.", len(files), list),
		fmt.Sprintf("%d dosyadaki kelime, satır ve karakterleri sayar: %s.", len(files), list))
}

// maxScreenBytes is how much the details panel reads. A screen has 15
// seconds (limits.call_timeout_s) and somebody is waiting; a bigger file
// is left to the job, which has minutes.
const maxScreenBytes = 2 << 20

// stats is a section of the details panel (placement "inspector") for the
// file the person is looking at: the counts at a glance.
func (a *app) stats(in *wire.ViewEventInput) (*wire.Surface, error) {
	say := func(text wire.Text, tone string) *wire.Surface {
		props := map[string]any{"text": text}
		if tone != "" {
			props["tone"] = tone
		}
		return &wire.Surface{Nodes: []wire.Node{{Type: "text", Props: props}}}
	}
	if len(in.Context.Inputs) == 0 {
		return say(t("Select a text file.", "Bir metin dosyası seçin."), "muted"), nil
	}
	f := in.Context.Inputs[0]
	if f.Size > maxScreenBytes {
		return say(t("This file is too large to count here. Use Word count… in its menu.",
			"Bu dosya burada sayılamayacak kadar büyük. Menüsündeki Kelime say… komutunu kullanın."), "muted"), nil
	}
	data, err := a.host.ReadInput(f.Ref)
	if err != nil {
		return nil, err
	}
	s, err := Count(f.Name, data, 5)
	if err != nil {
		return say(t("This file is not UTF-8 text.", "Bu dosya UTF-8 metin değil."), "muted"), nil
	}
	nodes := []wire.Node{{Type: "text", Props: map[string]any{"text": t(
		fmt.Sprintf("%s words · %s lines · %s characters", number("en", s.Words), number("en", s.Lines), number("en", s.Chars)),
		fmt.Sprintf("%s kelime · %s satır · %s karakter", number("tr", s.Words), number("tr", s.Lines), number("tr", s.Chars)),
	)}}}
	if len(s.Top) > 0 {
		en := make([]string, len(s.Top))
		tr := make([]string, len(s.Top))
		for i, w := range s.Top {
			en[i] = fmt.Sprintf("%s (%s)", w.Word, number("en", w.Count))
			tr[i] = fmt.Sprintf("%s (%s)", w.Word, number("tr", w.Count))
		}
		nodes = append(nodes, wire.Node{Type: "text", Props: map[string]any{"tone": "muted", "text": t(
			"Most frequent: "+strings.Join(en, ", "),
			"En sık: "+strings.Join(tr, ", "),
		)}})
	}
	out := &wire.Surface{Nodes: nodes, Actions: []wire.SurfaceAction{
		{ID: buttonReport, Label: t("Write a report…", "Rapor yaz…")},
	}}
	if in.Event == "action" && in.ActionID == buttonReport {
		// Open the file with "Word count…" started on it: the same dialog
		// the menu opens. filex checks the person may run it there.
		out.Open = &wire.OpenRequest{Path: f.Path, Action: actionCount}
	}
	return out, nil
}

func intPtr(n int) *int { return &n }
