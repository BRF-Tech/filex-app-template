#!/usr/bin/env bash
# Builds plugin.wasm, the module filex runs — after the tests pass.
#
#   bash scripts/build.sh            plugin.wasm + plugin.wasm.sha256
#   bash scripts/build.sh --stamp    also writes that sha256 into filex-app.json
#                                    (do this before tagging a release)
#
# filex refuses a module whose sha256 is not the one filex-app.json names,
# so a release is: build --stamp, commit filex-app.json, tag. The release
# workflow rebuilds at the tag and fails if the bytes differ — which is why
# every build here must produce the same bytes from the same source:
#
#   - the Go toolchain is the one go.mod pins (`toolchain goX.Y.Z`), forced
#     through GOTOOLCHAIN — another compiler version emits other bytes;
#   - -trimpath drops your machine's paths from the module;
#   - -buildvcs=false keeps the git commit out of it: otherwise the commit
#     that CARRIES the hash would change the hash.
#
# SKIP_TESTS=1 skips the tests; for bisecting a build problem, never for a
# release. OUT=path/to.wasm builds somewhere else.
set -euo pipefail
cd "$(dirname "$0")/.."

stamp=0
case "${1:-}" in
  "") ;;
  --stamp) stamp=1 ;;
  *) echo "usage: bash scripts/build.sh [--stamp]" >&2; exit 2 ;;
esac

pinned="$(sed -n 's/^toolchain //p' go.mod)"
if [ -z "$pinned" ]; then
  echo "go.mod has no 'toolchain goX.Y.Z' line: the build could not be reproduced elsewhere" >&2
  exit 1
fi
export GOTOOLCHAIN="$pinned"

if [ "$stamp" = 1 ] && grep -q '^replace ' go.mod; then
  echo "go.mod replaces a module with a local copy; a release must build against published modules" >&2
  exit 1
fi

out="${OUT:-plugin.wasm}"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# manifest.embed.json follows filex-app.json (see tools/manifest).
go generate ./...

if [ "${SKIP_TESTS:-0}" = 1 ]; then
  echo "tests skipped (SKIP_TESTS=1) — do not release this module" >&2
else
  echo "running the tests before building ($(go version))"
  if ! go test -count=1 ./...; then
    echo "build refused: the tests do not pass, so plugin.wasm is not produced" >&2
    exit 1
  fi
fi

GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -ldflags="-s -w" -buildmode=c-shared -o "$out" .

sum="$(sha256 "$out")"
size="$(wc -c < "$out" | tr -d ' ')"
echo "$sum  $(basename "$out")" > "$out.sha256"
printf '%s  %s bytes\nsha256       %s\n' "$out" "$size" "$sum"

if [ "$stamp" = 1 ]; then
  go run ./tools/manifest stamp "$out" >/dev/null
  stamped="$(grep -o '"sha256": *"[0-9a-f]\{64\}"' filex-app.json | grep -o '[0-9a-f]\{64\}')"
  if [ "$stamped" != "$sum" ]; then
    echo "stamp failed: filex-app.json says $stamped, the module is $sum" >&2
    exit 1
  fi
  echo "stamped wasm.sha256 into filex-app.json — commit it, then tag v$(sed -n 's/^ *"version": *"\([^"]*\)".*/\1/p' filex-app.json | head -1)"
fi
