# rust-minimal

A filex app in Rust, on the standard [Extism Rust PDK](https://github.com/extism/rust-pdk)
and nothing from filex: **Count lines (Rust)** on a text file writes
`<name>-lines.txt` beside it.

It is the proof that filex's ABI is not Go-only, and a reference for writing
the contract by hand: `src/lib.rs` exports `describe` and `action_run` and
imports `file_open`, `file_read`, `file_create`, `file_write` and
`file_close` from `extism:host/user`, including the two framed ones
(`file_read` answers `[status u8][bytes]`, `file_write` takes
`[handle u64 LE][bytes]`).

Build (no local Rust needed):

```bash
docker run --rm -v "$PWD":/src -w /src rust:1-slim sh -c \
  "rustup target add wasm32-unknown-unknown && cargo build --release --target wasm32-unknown-unknown"
```

Then install `target/wasm32-unknown-unknown/release/filex_rust_minimal.wasm`
with this `filex-app.json` through **Plugins → Apps → Install an app →
Upload files**.

What it leaves out: screens (`view_event`), and the `wasm` block a GitHub
install needs. `describe` embeds `filex-app.json` itself, which only works
while the manifest carries no hash of the module; an app published by URL
embeds a copy without the `wasm` block, as the Go template does
(`tools/manifest`).
