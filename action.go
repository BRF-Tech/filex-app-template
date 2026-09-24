package main

// The "Word count…" action. The menu row opens the options screen first
// (`"view": "options"` in filex-app.json); its submit queues this job with
// the values the person chose. A job is the only call that may write files.

import (
	"fmt"
	"path"
	"strings"

	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

// maxInputBytes keeps a file within the module's memory: the manifest asks
// for 1024 pages (64 MiB), and the file, its text and the word table all
// live there at once. Raise both together if your app needs more.
const maxInputBytes = 16 << 20

// params is what the options screen collects and the job receives. The
// job checks it again: a screen is a convenience, the job is the boundary
// (anyone allowed to run the action can call it with any params).
type params struct {
	Format  string // formatText | formatMarkdown
	Top     int    // how many frequent words to list, 0..maxTop
	Combine bool   // one report for the whole selection
}

const (
	defaultTop = 10
	maxTop     = 50
)

func defaultParams() params { return params{Format: formatText, Top: defaultTop} }

// readParams reads the values a form or a job carries. JSON numbers
// arrive as float64 and a form may send an int as a string, so both are
// accepted. It answers the problems per field, in both languages.
func readParams(v map[string]any) (params, map[string]wire.Text) {
	p := defaultParams()
	errs := map[string]wire.Text{}
	if f, ok := v["format"].(string); ok && f != "" {
		p.Format = f
	}
	if p.Format != formatText && p.Format != formatMarkdown {
		errs["format"] = t("Choose a report format.", "Bir rapor biçimi seçin.")
	}
	if raw, ok := v["top"]; ok && raw != nil && raw != "" {
		n, ok := asInt(raw)
		if !ok || n < 0 || n > maxTop {
			errs["top"] = t(fmt.Sprintf("Enter a number from 0 to %d.", maxTop),
				fmt.Sprintf("0 ile %d arasında bir sayı girin.", maxTop))
		} else {
			p.Top = n
		}
	}
	p.Combine, _ = v["combine"].(bool)
	return p, errs
}

func (p params) toMap() map[string]any {
	return map[string]any{"format": p.Format, "top": p.Top, "combine": p.Combine}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), n == float64(int(n))
	case int:
		return n, true
	case string:
		var i int
		_, err := fmt.Sscan(strings.TrimSpace(n), &i)
		return i, err == nil
	}
	return 0, false
}

// count is the job. It reads every selected file, writes one report per
// file (or one for all of them), and says what it did in the tray.
func (a *app) count(in *wire.ActionRunInput) (*wire.ActionRunOutput, error) {
	p, errs := readParams(in.Params)
	for _, key := range []string{"format", "top"} {
		if e, bad := errs[key]; bad {
			return &wire.ActionRunOutput{OK: false, Message: e}, nil
		}
	}
	var (
		counted []Stats
		skipped []string
		words   int
	)
	for i, f := range in.Inputs {
		a.host.Progress(int64(i), int64(len(in.Inputs)), f.Name)
		if f.Size > maxInputBytes {
			skipped = append(skipped, f.Name)
			a.host.Log("warn", fmt.Sprintf("%s: %d bytes, above the %d this app reads", f.Name, f.Size, maxInputBytes))
			continue
		}
		data, err := a.host.ReadInput(f.Ref)
		if err != nil {
			// A host error is in band (*pluginkit.HostError): returning it
			// fails the job and shows its message in the tray.
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		s, err := Count(f.Name, data, p.Top)
		if err != nil {
			skipped = append(skipped, f.Name)
			continue
		}
		counted = append(counted, s)
		words += s.Words
	}
	a.host.Progress(int64(len(in.Inputs)), int64(len(in.Inputs)), "")

	if len(counted) == 0 {
		return &wire.ActionRunOutput{OK: false, Message: t(
			"Nothing to count: "+strings.Join(skipped, ", ")+" is not UTF-8 text or is too large.",
			"Sayılacak bir şey yok: "+strings.Join(skipped, ", ")+" UTF-8 metin değil ya da çok büyük.")}, nil
	}

	// The outputs. Their names are the app's: filex writes each one beside
	// the first selected file (output mode "sibling") and picks a free name
	// ("…-copy") when that one is taken, so nothing is overwritten.
	ext := "." + p.Format
	var outs []wire.OutputRef
	if p.Combine || len(counted) == 1 {
		name := "wordcount-report" + ext
		if len(counted) == 1 {
			name = stem(counted[0].Name) + "-wordcount" + ext
		}
		ref, err := a.host.WriteOutput(name, Report(in.Locale, p.Format, counted))
		if err != nil {
			return nil, err
		}
		outs = append(outs, ref)
	} else {
		for _, s := range counted {
			ref, err := a.host.WriteOutput(stem(s.Name)+"-wordcount"+ext, Report(in.Locale, p.Format, []Stats{s}))
			if err != nil {
				return nil, err
			}
			outs = append(outs, ref)
		}
	}

	files := "1 file"
	if len(counted) > 1 {
		files = fmt.Sprintf("%d files", len(counted))
	}
	msg := t(fmt.Sprintf("Counted %s words in %s.", number("en", words), files),
		fmt.Sprintf("%d dosyada %s kelime sayıldı.", len(counted), number("tr", words)))
	if len(skipped) > 0 {
		msg = t(msg["en"]+" Skipped: "+strings.Join(skipped, ", ")+".",
			msg["tr"]+" Atlananlar: "+strings.Join(skipped, ", ")+".")
	}
	return &wire.ActionRunOutput{OK: true, Outputs: outs, Message: msg}, nil
}

func stem(name string) string { return strings.TrimSuffix(name, path.Ext(name)) }
