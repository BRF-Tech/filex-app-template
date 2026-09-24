//! A filex app in Rust, on the standard Extism PDK — no filex SDK.
//!
//! It speaks filex's ABI by hand, which is the whole contract
//! (docs/PLUGIN-KIT.md → "Host functions" and "Other languages"):
//!
//! - exports `describe` and `action_run`, JSON in and out through Extism's
//!   input/output buffers (an app with views also exports `view_event`);
//! - imports host functions from the `extism:host/user` namespace, one
//!   Extism memory pointer in and one out. All are JSON, except the two
//!   framed chunk functions: `file_read` answers `[status u8][bytes]`
//!   (0 data, 1 end of file, 2 error JSON) and `file_write` takes
//!   `[handle u64 little-endian][bytes]`;
//! - a refused call is answered in band, `{"error": {"code", "message"}}`.

use extism_pdk::*;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

/// What `describe` answers: the manifest filex installed, minus nothing
/// (this example is installed by upload, so its manifest has no `wasm`
/// block; see the template's tools/manifest for an app published by URL).
const MANIFEST: &str = include_str!("../filex-app.json");

#[host_fn("extism:host/user")]
extern "ExtismHost" {
    fn file_open(input: String) -> String;
    fn file_read(input: String) -> Vec<u8>;
    fn file_create(input: String) -> String;
    fn file_write(input: Vec<u8>) -> String;
    fn file_close(input: String) -> String;
}

#[derive(Deserialize)]
struct FileRef {
    #[serde(rename = "ref")]
    reference: String,
    name: String,
}

#[derive(Deserialize)]
struct ActionRunInput {
    inputs: Vec<FileRef>,
}

#[derive(Serialize)]
struct OutputRef {
    #[serde(rename = "ref")]
    reference: String,
    name: String,
}

#[derive(Serialize)]
struct ActionRunOutput {
    ok: bool,
    outputs: Vec<OutputRef>,
    message: Value,
}

/// A JSON host call: an in-band `{"error": …}` becomes an Err.
fn host_json(answer: String) -> Result<Value, Error> {
    let v: Value = serde_json::from_str(&answer)?;
    if let Some(e) = v.get("error") {
        return Err(Error::msg(format!("filex refused: {e}")));
    }
    Ok(v)
}

fn read_all(reference: &str) -> Result<Vec<u8>, Error> {
    let opened = host_json(unsafe { file_open(json!({ "ref": reference }).to_string())? })?;
    let handle = opened["handle"].as_u64().ok_or_else(|| Error::msg("file_open: no handle"))?;
    let mut data = Vec::new();
    loop {
        let frame = unsafe { file_read(json!({ "handle": handle, "max": 1 << 20 }).to_string())? };
        match frame.first() {
            Some(0) => data.extend_from_slice(&frame[1..]),
            Some(1) => break,
            _ => return Err(Error::msg(format!("file_read: {}", String::from_utf8_lossy(frame.get(1..).unwrap_or(&[]))))),
        }
    }
    host_json(unsafe { file_close(json!({ "handle": handle }).to_string())? })?;
    Ok(data)
}

fn write_all(name: &str, data: &[u8]) -> Result<OutputRef, Error> {
    let created = host_json(unsafe { file_create(json!({ "name": name }).to_string())? })?;
    let handle = created["handle"].as_u64().ok_or_else(|| Error::msg("file_create: no handle"))?;
    let reference = created["ref"].as_str().unwrap_or_default().to_string();
    for chunk in data.chunks(1 << 20) {
        let mut frame = handle.to_le_bytes().to_vec();
        frame.extend_from_slice(chunk);
        host_json(unsafe { file_write(frame)? })?;
    }
    host_json(unsafe { file_close(json!({ "handle": handle }).to_string())? })?;
    Ok(OutputRef { reference, name: name.to_string() })
}

#[plugin_fn]
pub fn describe(_input: String) -> FnResult<String> {
    Ok(MANIFEST.to_string())
}

#[plugin_fn]
pub fn action_run(Json(input): Json<ActionRunInput>) -> FnResult<Json<ActionRunOutput>> {
    let file = input.inputs.first().ok_or_else(|| Error::msg("no input file"))?;
    let data = read_all(&file.reference)?;
    let text = String::from_utf8_lossy(&data);
    let lines = text.lines().count();
    let stem = file.name.rsplit_once('.').map(|(s, _)| s).unwrap_or(&file.name);
    let report = format!("{}: {} lines, {} bytes (counted by a Rust app)\n", file.name, lines, data.len());
    let out = write_all(&format!("{stem}-lines.txt"), report.as_bytes())?;
    Ok(Json(ActionRunOutput {
        ok: true,
        outputs: vec![out],
        message: json!({
            "en": format!("{} has {} lines.", file.name, lines),
            "tr": format!("{} dosyasında {} satır var.", file.name, lines),
        }),
    }))
}
