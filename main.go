// Command wordcount is the example app of the filex app template: a small
// app that does real work, written the way a larger one should be.
//
//   - filex-app.json   what the app is and what it asks for (the only file
//     the administrator reviews). This file registers what it offers.
//   - action.go        "Word count…" in the file menu: a job that reads the
//     selected text files and writes a report beside them.
//   - views.go         two screens: the options dialog the action opens
//     first, and a section in a file's details panel.
//   - count.go         the counting and the report, plain Go with no filex
//     in it — the part you replace with your own app.
//
// Build it with scripts/build.sh; it is a WebAssembly module
// (GOOS=wasip1 GOARCH=wasm -buildmode=c-shared) that filex runs in its
// sandbox. `go test ./...` runs everything below against a fake filex
// (pluginkit/plugintest) without building the module.
package main

import (
	_ "embed"
	"encoding/json"

	"github.com/brf-tech/filex/backend/pkg/pluginkit"
	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

// The ids the manifest declares. Rename them together with filex-app.json;
// TestEveryIDIsRegistered fails when the two disagree.
const (
	actionCount = "count"
	viewOptions = "options"
	viewStats   = "stats"
)

// manifest.embed.json is filex-app.json without its `wasm` block, written
// by `go generate` (tools/manifest). Edit filex-app.json, never this copy.
//
//go:generate go run ./tools/manifest embed
//go:embed manifest.embed.json
var manifestJSON []byte

// Manifest is what the `describe` export answers. filex compares it with
// the filex-app.json the administrator approved and refuses the app when
// the name, version or permissions differ, so it is read from that file
// rather than written a second time in Go.
func Manifest() wire.Manifest {
	var m wire.Manifest
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		panic("manifest.embed.json is not a valid manifest: " + err.Error())
	}
	return m
}

// newPlugin assembles the app over a host. The module passes the real one
// (sdkHost, below); the tests pass plugintest's fake filex. Add your own
// actions and views here, under the ids filex-app.json declares.
func newPlugin(h Host) *pluginkit.Plugin {
	a := &app{host: h}
	return &pluginkit.Plugin{
		Manifest: Manifest(),
		Actions: map[string]pluginkit.ActionFunc{
			actionCount: a.count,
		},
		Views: map[string]pluginkit.ViewFunc{
			viewOptions: a.options,
			viewStats:   a.stats,
		},
	}
}

// ⚠ Register from init(), not from main(). The module is built with
// -buildmode=c-shared, which makes it a *reactor*: filex calls its exports
// (describe, action_run, view_event, …) directly, package initialisers
// run, and main() never does. An app that registers in main() answers
// every call with "no plugin registered".
func init() { pluginkit.Run(newPlugin(sdkHost{})) }

func main() {}

type app struct{ host Host }

// t is one sentence in every language filex-app.json declares
// ("languages": ["en", "tr"]). A screen sends the whole Text and the
// browser shows the reader's language; a file the app writes picks one
// with .Get(lang), which falls back to English. Declaring a third
// language means a third argument here — plugintest.CheckLanguages fails
// every screen that is missing it, and filex refuses to install the app.
func t(en, tr string) wire.Text { return wire.Text{"en": en, "tr": tr} }

// Host is the part of filex this app talks to — one method per host
// function it calls, with pluginkit's own names and signatures, so
// *plugintest.Host satisfies it as it is. Taking the host as a parameter
// is what lets `go test` run the app: pluginkit's functions only answer
// inside filex (off-wasm they return pluginkit.ErrNotWasm).
//
// When your app needs another host function (StateGet, EngineRun,
// NotifySend, …), add it here and to sdkHost, and add the permission it
// needs to filex-app.json.
type Host interface {
	// ReadInput reads a selected file (permission files:read).
	ReadInput(ref string) ([]byte, error)
	// WriteOutput creates a result file (permission files:write; jobs only).
	WriteOutput(name string, data []byte) (wire.OutputRef, error)
	// Progress draws the job's bar in the operations tray (no permission).
	Progress(done, total int64, message string)
	// Log writes a line to the app's log in Admin → Plugins → Apps.
	Log(level, msg string)
}

// sdkHost is the real filex, reached through pluginkit.
type sdkHost struct{}

func (sdkHost) ReadInput(ref string) ([]byte, error) { return pluginkit.ReadInput(ref) }

func (sdkHost) WriteOutput(name string, data []byte) (wire.OutputRef, error) {
	return pluginkit.WriteOutput(name, data)
}

func (sdkHost) Progress(done, total int64, message string) {
	pluginkit.Progress(done, total, message)
}

func (sdkHost) Log(level, msg string) { pluginkit.Log(level, msg) }
