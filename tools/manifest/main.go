// Command manifest keeps filex-app.json and the module in step.
//
//	go run ./tools/manifest embed               filex-app.json -> manifest.embed.json (run by go generate)
//	go run ./tools/manifest stamp plugin.wasm   writes the module's sha256 into filex-app.json
//	go run ./tools/manifest rename <name> <owner/repo> [label-en] [label-tr]
//	                                            used once by scripts/rename.sh
//
// Why two files: filex-app.json carries `wasm.sha256`, the hash of the
// module. If the module embedded filex-app.json itself, writing the hash
// would change the module, which would change the hash, for ever. So the
// module embeds manifest.embed.json, which is filex-app.json WITHOUT its
// `wasm` block: stamping cannot change the bytes it describes, and
// TestManifestEmbedIsFresh fails the day the two drift.
//
// You never edit manifest.embed.json. Edit filex-app.json and run
// `go generate ./...` (scripts/build.sh does it for you).
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/brf-tech/filex/backend/pkg/pluginkit/wire"
)

const (
	manifestFile = "filex-app.json"
	embedFile    = "manifest.embed.json"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 2 && os.Args[1] == "embed":
		err = embed()
	case len(os.Args) == 3 && os.Args[1] == "stamp":
		err = stamp(os.Args[2])
	case len(os.Args) >= 4 && len(os.Args) <= 6 && os.Args[1] == "rename":
		err = rename(os.Args[2], os.Args[3], append(os.Args[4:], "", "")[:2])
	default:
		err = errors.New("usage: go run ./tools/manifest embed | stamp <plugin.wasm> | rename <name> <owner/repo> [label-en] [label-tr]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "manifest:", err)
		os.Exit(1)
	}
}

// embed writes manifest.embed.json: every top-level member of
// filex-app.json, in its order and with its own formatting, except `wasm`.
func embed() error {
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	if err := validate(src); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return fmt.Errorf("%s is not a JSON object", manifestFile)
	}
	var out bytes.Buffer
	out.WriteString("{")
	first := true
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("%s: %q: %w", manifestFile, key, err)
		}
		if key == "wasm" {
			continue
		}
		if !first {
			out.WriteString(",")
		}
		first = false
		k, _ := json.Marshal(key)
		fmt.Fprintf(&out, "\n  %s: %s", k, raw)
	}
	out.WriteString("\n}\n")
	return os.WriteFile(embedFile, out.Bytes(), 0o644)
}

// validate decodes the manifest the way filex does, refusing a key filex
// does not know: a typo such as "permisions" would otherwise install an app
// with no grants, and every host call it makes would be refused.
func validate(src []byte) error {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	var m wire.Manifest
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("%s: %w", manifestFile, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("%s: trailing data after the manifest", manifestFile)
	}
	if m.Name == "" || m.Version == "" {
		return fmt.Errorf("%s: name and version are required", manifestFile)
	}
	return nil
}

var shaField = regexp.MustCompile(`("sha256"\s*:\s*")[0-9a-f]{64}(")`)

// stamp replaces the 64-hex value of `wasm.sha256` in place, leaving every
// other byte of the file as it was.
func stamp(wasmPath string) error {
	mod, err := os.ReadFile(wasmPath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(mod)
	hexSum := hex.EncodeToString(sum[:])
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	if n := len(shaField.FindAllIndex(src, -1)); n != 1 {
		return fmt.Errorf("%s: expected exactly one \"sha256\": \"<64 hex>\" (the wasm block), found %d", manifestFile, n)
	}
	out := shaField.ReplaceAll(src, []byte("${1}"+hexSum+"${2}"))
	if err := os.WriteFile(manifestFile, out, 0o644); err != nil {
		return err
	}
	fmt.Println(hexSum)
	return nil
}

// The template's own values, which rename replaces.
const (
	templateName  = `"name": "wordcount"`
	templateRepo  = "https://github.com/BRF-Tech/filex-app-template"
	templateLabel = `"label": { "en": "Word count", "tr": "Kelime sayacı" }`
)

var (
	appName     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	repoName    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	versionLine = regexp.MustCompile(`("version"\s*:\s*")[^"]*(")`)
)

// rename turns the template's manifest into a new app's: its name, its
// repository (homepage and the module's download address), a first
// version, no hash yet, and — when given — its label.
func rename(name, repo string, label []string) error {
	if !appName.MatchString(name) {
		return fmt.Errorf("app name %q: use 1-32 of a-z, 0-9, _ and -, starting with a letter or digit", name)
	}
	switch name {
	case "cache", "spool", "public", "assets":
		return fmt.Errorf("app name %q is one of filex's own directories", name)
	}
	if !repoName.MatchString(repo) {
		return fmt.Errorf("repository %q: expected owner/name", repo)
	}
	src, err := os.ReadFile(manifestFile)
	if err != nil {
		return err
	}
	s := string(src)
	for _, want := range []string{templateName, templateRepo} {
		if !strings.Contains(s, want) {
			return fmt.Errorf("%s no longer holds the template's %s; rename runs once, on a fresh copy of the template", manifestFile, want)
		}
	}
	s = strings.Replace(s, templateName, `"name": "`+name+`"`, 1)
	s = strings.ReplaceAll(s, templateRepo, "https://github.com/"+repo)
	s = versionLine.ReplaceAllString(s, "${1}0.1.0${2}")
	s = shaField.ReplaceAllString(s, "${1}"+strings.Repeat("0", 64)+"${2}")
	if label[0] != "" {
		l := map[string]string{"en": label[0], "tr": label[1]}
		if l["tr"] == "" {
			l["tr"] = label[0]
		}
		en, _ := json.Marshal(l["en"])
		tr, _ := json.Marshal(l["tr"])
		if !strings.Contains(s, templateLabel) {
			return fmt.Errorf("%s: the template's label line is not there to replace", manifestFile)
		}
		s = strings.Replace(s, templateLabel, fmt.Sprintf(`"label": { "en": %s, "tr": %s }`, en, tr), 1)
	}
	if err := validate([]byte(s)); err != nil {
		return err
	}
	return os.WriteFile(manifestFile, []byte(s), 0o644)
}
