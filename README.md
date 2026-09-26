# filex app template

Start a **filex app** — a WebAssembly module that adds actions to the file
menu of [filex](https://github.com/BRF-Tech/filex), the self-hosted file
manager, and runs them as jobs inside filex's sandbox with only the
permissions an administrator granted.

This repository is a small app that does real work, plus everything around
it that a published app needs:

- **Word count** (`wordcount`): right-click one or more text files →
  *Word count…* → a dialog asks for the report format and how many frequent
  words to list → a job writes `notes-wordcount.txt` (or `.md`) next to each
  file, or one report for all of them. A section in the file's details panel
  shows the counts at a glance. English and Turkish throughout.
- **Tests that run without filex**: `go test` drives the app against
  `plugintest`, the fake filex that ships with the SDK, with the manifest's
  permissions as the grant.
- **A reproducible build** (`scripts/build.sh`), a **rename script**, and
  **GitHub Actions** that test every pull request and publish a release on a
  version tag.

It is written in Go against the official SDK,
[`pkg/pluginkit`](https://github.com/BRF-Tech/filex/tree/main/backend/pkg/pluginkit).
Other languages are possible — see
[Which languages can I write an app in?](#which-languages-can-i-write-an-app-in)

The contract every app follows is documented in filex:
[Writing an app plugin](https://github.com/BRF-Tech/filex/blob/main/docs/PLUGIN-KIT.md) (manifest, exports,
host functions, screens, test kit) and
[Apps](https://github.com/BRF-Tech/filex/blob/main/docs/APP-PLUGINS.md) (what an administrator sees).

---

## What is in here

```
filex-app.json          the manifest: what the app is, what it asks for, what it offers
main.go                 registers the app (from init()), reads the manifest, the Host interface
action.go               the "Word count…" job: reads the files, writes the reports
views.go                the options dialog and the details-panel section
count.go                the counting itself — plain Go, no filex in it
main_test.go            the tests, against plugintest's fake filex
testdata/golden/        the stored shape of each screen (plugintest.Golden)
manifest.embed.json     generated: filex-app.json without its wasm block (go generate)
tools/manifest/         embeds, stamps and renames the manifest
scripts/build.sh        tests, then builds plugin.wasm; --stamp writes its sha256
scripts/rename.sh       turns the template into your app
examples/rust-minimal/  the same ABI from Rust, without the Go SDK
.github/workflows/      CI (tests + build) and Release (on a v* tag)
```

## 1. Make it yours

Click **Use this template** on GitHub (or copy the repository), then give
the app its name and its repository:

```bash
bash scripts/rename.sh invoices you/filex-invoices "Invoice tools" "Fatura araçları"
```

- `invoices` is the id filex knows the app by: 1–32 of `a-z`, `0-9`, `_`,
  `-`. It names the app's directory on the server and every menu key
  (`plugin:invoices/<action>`), so choose it once.
- `you/filex-invoices` is the GitHub repository you will publish from. It
  becomes the manifest's `homepage` and the address filex downloads the
  module from (`…/releases/download/{tag}/plugin.wasm`), and the Go module
  path.
- The labels are what the file menu and the install review show, in English
  and Turkish.

The script leaves the word count in place — it is what you replace next.

You need **Go 1.25 or newer**. The build pins its compiler exactly (see
[Reproducible builds](#reproducible-builds)); Go downloads that version by
itself the first time.

## 2. Write the app

Start with `filex-app.json` — it is the only file the administrator reviews,
and everything the app does has to be declared there first:

| Key | Here | What it does |
|---|---|---|
| `permissions` | `files:read`, `files:write` | the whole grant. A host call outside it is refused |
| `permission_reasons` | one sentence each | shown beside each permission at install — say why |
| `languages` | `en`, `tr` | every text the app returns must carry all of them |
| `actions[]` | `count` | a file-menu row: which files (`applies`), the dialog it opens first (`view`), who may run it (`min_role`), where results go (`output`) |
| `views[]` | `options` (modal), `stats` (inspector) | screens: a dialog, a section of the details panel, a full page or a home screen |
| `limits` | 64 MiB, 15 s | memory and a screen's time budget; a job gets `limits.timeout_s` |
| `min_filex` / `filex` | `min_filex: 0.43.0` | which filex versions the app works with — see [Which filex your app works with](#which-filex-your-app-works-with) |
| `wasm` | `url`, `sha256` | where the module is downloaded from and its hash; `build.sh --stamp` writes the hash |

The full list of fields, permissions, node types and host functions is in
[Writing an app plugin](https://github.com/BRF-Tech/filex/blob/main/docs/PLUGIN-KIT.md).

Then the Go side:

- **`main.go`** registers the actions and views with `pluginkit.Run` — from
  `init()`, not `main()`. The module is built with `-buildmode=c-shared`,
  which makes it a *reactor*: filex calls its exports directly and `main()`
  never runs.
- **The manifest is read, not repeated.** `describe` answers the manifest
  filex compares with the one the administrator approved; this app reads it
  from `manifest.embed.json`, a copy of `filex-app.json` without the `wasm`
  block that `go generate` writes (the module cannot embed its own hash).
  Edit `filex-app.json` only; `TestManifestEmbedIsFresh` and
  `TestDescribeIsTheManifest` fail when the two drift.
- **The host is a parameter.** The app calls filex through the small `Host`
  interface in `main.go`. The module passes the real filex (`sdkHost`,
  `pluginkit`'s functions); the tests pass `plugintest`'s fake. When you need
  another host function — `StateGet`, `EngineRun`, `NotifySend`, … — add it
  to the interface and to `sdkHost`, and its permission to the manifest.
- **Screens read, jobs write.** A view may read the selected files but never
  write; it answers a `Job` and filex queues the action, which is the only
  call that may create files. A job checks its parameters again: anyone
  allowed to run the action can call it with any parameters.
- **Two languages, everywhere.** A `wire.Text` (`t("…", "…")` here) carries
  both and the browser shows the reader's; a form field's label is one
  string in the call's language (`.Get(lang)`). A file the app writes is in
  the language of the person who ran it.

## 3. Test

```bash
go test ./...
go test ./... -update     # after an intended change to a screen; review `git diff testdata/golden`
```

The tests never build the module. `plugintest` runs the app's own functions
behind a fake filex that refuses what filex refuses — a permission the
manifest did not ask for, a file written from a screen — and checks what
filex checks at install:

| Check | Catches |
|---|---|
| `CheckManifest`, `CheckManifestLanguages`, `CheckRegistered` | a permission filex does not know, an undeclared view, a text missing a language, an id with no function |
| `CheckSurface` | a node filex cannot draw, a choice without options, two primary buttons, conditions on missing fields |
| `CheckLanguages`, `CheckLocaleParityOpts` | a screen half in one language — drawn in English and Turkish side by side |
| `Golden` | a screen that changed shape without anybody meaning it to |

## 4. Build

```bash
bash scripts/build.sh
```

It regenerates `manifest.embed.json`, runs the tests (and refuses to build
when they fail), then builds `plugin.wasm` and `plugin.wasm.sha256`:

```bash
GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -ldflags="-s -w" -buildmode=c-shared -o plugin.wasm .
```

The example comes out at about 6 MB.

### Reproducible builds

filex refuses a module whose sha256 is not the one `filex-app.json` names,
and the release workflow rebuilds the module from the tag and compares. So
the same source must give the same bytes on your machine and on GitHub's:

- **the compiler** is the one `go.mod` pins with `toolchain goX.Y.Z`;
  `build.sh` forces it through `GOTOOLCHAIN` (a different Go version emits
  different bytes). Bump that line to move to a newer Go;
- **`-trimpath`** keeps your directories out of the module;
- **`-buildvcs=false`** keeps the git commit out of it — otherwise the commit
  that carries the hash would change the hash.

Measured: the same commit built in WSL and in a clean `golang` container
(another Go preinstalled, empty module cache, another path) produced the
same sha256.

## 5. Try it on your own filex

Any filex **0.43.0** or newer. For a throwaway one, download the binary for
your system from the [filex releases](https://github.com/BRF-Tech/filex/releases)
and start it with a data directory of its own:

```bash
FILEX_DATA_DIR=./filex-data FILEX_LISTEN=127.0.0.1:5212 \
FILEX_ADMIN_EMAIL=admin@local FILEX_ADMIN_PASSWORD=admin \
./filex-linux-amd64 serve
```

Then, at `http://127.0.0.1:5212/admin`:

1. **Storages** → add a *local* storage pointing at a folder with a few text
   files.
2. **Plugins → Apps → Install an app → Upload files**: `plugin.wasm` and
   `filex-app.json`. The review lists every permission with your reason
   beside it; tick *I understand…* and **Install**. The app's row says
   *Running*.
3. In the file explorer, right-click a `.txt` file → **Word count…** →
   **Count**. The report appears beside the file, and the operations tray says
   what was counted. Select a file and open **Details** to see the panel
   section.

To try a new build, use **Upgrade** on the app's row (or remove it and
install again).

## 6. Publish

1. Bump `version` in `filex-app.json`.
2. `bash scripts/build.sh --stamp` — builds and writes the module's sha256
   into `filex-app.json`.
3. Commit, tag `v<version>`, push the tag.

The **Release** workflow checks that the tag matches the manifest's version
and that `wasm.url` points at this repository, builds the module again, fails
unless it hashes to what you committed, and attaches `plugin.wasm` to the
GitHub release.

An administrator then installs it with **Plugins → Apps → Install an app →
GitHub repository**: `you/filex-invoices` and the tag (`v0.1.0`). filex reads
`filex-app.json` at that tag, downloads the module from `wasm.url` (with
`{tag}` replaced) and refuses it unless the sha256 matches. ⚠ Give the tag,
not a branch: the module lives under a release.

To ship an update, bump the version and tag again. From filex 0.47 an app
installed from GitHub **follows its releases**: once a day (or when an
administrator presses **Check for updates**) filex reads the newest GitHub
*release* — not a draft, not a pre-release — whose `filex-app.json` at that tag
names the app and whose `filex` range lets that server in, and installs it by
itself when it asks for **no new permission**. An update that asks for a new
permission is never installed by itself: it waits on the Apps tab as *Needs
approval* until an administrator reviews it, and the old version keeps
running meanwhile. So:

- publish a GitHub **release** for every tag (the Release workflow does) —
  a tag without a release is not seen;
- keep `version` equal to the tag, and never move a tag after it is
  published: servers pin the module by the hash the manifest said;
- keep the permissions if you want the update to arrive by itself;
- say the filex versions a release needs (below), so servers that have not
  upgraded keep the version that works for them.

Details: [Updates](https://github.com/BRF-Tech/filex/blob/main/docs/APP-PLUGINS.md#updates)
and [Publishing so updates are found](https://github.com/BRF-Tech/filex/blob/main/docs/PLUGIN-KIT.md#publishing-so-updates-are-found).

## Which filex your app works with

`filex` in `filex-app.json` is a range of filex versions, written as a small
subset of the npm/Cargo syntax — comparators joined by a space must all hold,
alternatives are joined by `||`, versions are `MAJOR.MINOR.PATCH`:

```json
"filex": ">=0.47.0"
"filex": ">=0.47.0 <0.60.0"
```

filex refuses to install or upgrade to a version whose range leaves it out
(the install review says so first), and its update check takes the newest
release whose range fits. The older `min_filex` (`"0.43.0"` = `>=0.43.0`) is
still honoured. The Rust example ([examples/rust-minimal](examples/rust-minimal/filex-app.json))
declares `"filex": ">=0.47.0"`.

⚠ This template's own `filex-app.json` still says `min_filex`: filex before
0.47 — and the SDK this template builds against (`go.mod`: backend v0.45.1),
whose manifest tests decode `filex-app.json` the way filex does
(`readManifestFile` in `main_test.go`) — refuse a manifest with a field they
do not know. Once `go.mod` requires
backend v0.47.0 or later, replace `"min_filex": "0.43.0"` with
`"filex": ">=0.47.0"` (and run `go generate ./...`). Keep `min_filex` for as
long as your app must also install on filex 0.43–0.46.

## The permission model

An app runs inside filex's process, in a WebAssembly sandbox with no
filesystem and no network of its own. Everything it does outside that
sandbox is a host function, and every host function is gated by a
permission:

- The manifest's `permissions` is the **whole** list. Installing grants
  exactly that list — no partial grants — and filex refuses a module whose
  `describe` asks for more than the approved manifest.
- A refused call is answered in band (`permission_denied`), not with a crash,
  so an app can degrade and say so.
- **Screens are read-only.** Only an action's job may write files, take locks,
  sign or open public links.
- The administrator can switch actions off, reserve them to administrators,
  narrow where they apply, and read the app's log in **Plugins → Apps**.

Ask for the least you need, and write a reason a stranger would accept. This
app asks for two:

| Permission | Why the word count needs it |
|---|---|
| `files:read` | to read the text files it is run on (the details-panel section reads the file too) |
| `files:write` | to write the report next to them — nothing else is changed |

The other permissions (`state`, `settings`, `engines:<name>`, `users:lookup`,
`notify:send`, `mail:send`, `http:<host>`, `public_pages`, `sign`,
`files:lock`, `schedule`) are listed with what each unlocks in
[Writing an app plugin → Permissions](https://github.com/BRF-Tech/filex/blob/main/docs/PLUGIN-KIT.md#permissions).

## Which languages can I write an app in?

filex runs apps with the [Extism](https://extism.org) runtime: an app is a
WebAssembly module that exports `describe`, `action_run`, `view_event`, … and
imports filex's host functions from the `extism:host/user` namespace, JSON in
and out through Extism's memory. Any language with an Extism PDK can speak
that — but not all of them have been tried.

| Language | Status |
|---|---|
| **Go** | **Official SDK** — `pkg/pluginkit`: typed wire structs for the manifest and every screen node, wrappers for every host function, and the `plugintest` test kit. This template, and the apps that ship with filex ([e-Signature](https://github.com/BRF-Tech/filex-sign), [Convert](https://github.com/BRF-Tech/filex-convert)), are written in it. Stock Go, no TinyGo. |
| **Rust** | **Measured** with [`extism-pdk`](https://github.com/extism/rust-pdk) 1.4: [`examples/rust-minimal`](examples/rust-minimal) exports `describe` and `action_run`, reads its input with `file_open` / `file_read` / `file_close` and writes a result with `file_create` / `file_write` / `file_close`. Built for `wasm32-unknown-unknown` (about 150 KB), installed on filex 0.43.0 through the permission review, and run from the file menu and the API — including a 4.2 MB file read in 1 MiB chunks. Screens (`view_event`) were not part of the test. |
| JavaScript / TypeScript, AssemblyScript, C, C++, Zig, C# / F# (.NET), Haskell, Python, MoonBit | **Not tried.** Each has an Extism PDK ([list](https://extism.org/docs/concepts/pdk); Python's is at 0.1.x, MoonBit's has no release yet), so each can in principle import the same host functions and export the same functions. Nobody has built a filex app in them yet. |

Outside Go you write the contract yourself, from the tables in
[Writing an app plugin](https://github.com/BRF-Tech/filex/blob/main/docs/PLUGIN-KIT.md) (the Rust example
is a working reference):

- `describe` must answer the manifest you install, with every text in every
  language the manifest declares;
- host functions take and return one Extism memory pointer. All are JSON
  except two framed ones: `file_read` answers `[status u8][bytes]` (0 data,
  1 end of file, 2 an error in JSON), `file_write` takes
  `[handle u64 little-endian][bytes]`;
- a failure comes back in band as `{"error": {"code", "message"}}`;
- there is no `plugintest`: you test by installing the module.

## Troubleshooting

| Symptom | Cause |
|---|---|
| Install answers `sha256_mismatch` | the module is not the one `filex-app.json` names: run `build.sh --stamp` and commit, or the release asset and the manifest at that tag disagree |
| Install answers `describe_mismatch` | the module was built from another manifest (name, version, permissions): rebuild after editing `filex-app.json` |
| Every call fails with *no plugin registered* | `pluginkit.Run` is called from `main()`; call it from `init()` |
| A host call fails with `permission_denied` | the permission is not in `filex-app.json` — or the call wrote from a screen, which only a job may do |
| A GitHub install answers `fetch_failed` | give the release's tag, not a branch |
| The release workflow says the hash differs | the module was stamped with another compiler: keep `toolchain` in `go.mod` and build with `scripts/build.sh` |

## License

[MIT](LICENSE).
