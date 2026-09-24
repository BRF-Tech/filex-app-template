#!/usr/bin/env bash
# Turns this template into your app. Run it once, on a fresh copy:
#
#   bash scripts/rename.sh <app-name> <owner/repo> ["Label"] ["Turkish label"]
#   bash scripts/rename.sh invoices acme/filex-invoices "Invoice tools" "Fatura araçları"
#
# <app-name> is the id filex knows your app by: 1-32 of a-z, 0-9, _ and -.
# It names the app's directory on the server and every menu key
# (`plugin:<name>/<action>`), so pick it once. <owner/repo> is the GitHub
# repository you will publish from: it becomes the homepage and the
# address filex downloads the module from (…/releases/download/{tag}/plugin.wasm).
#
# It rewrites filex-app.json (name, homepage, wasm.url, version 0.1.0, an
# empty hash, the label) and the Go module path, then runs the tests. The
# example's actions and screens stay: they are what you replace next.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ $# -lt 2 ] || [ $# -gt 4 ]; then
  sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
fi
name="$1"
repo="$2"

pinned="$(sed -n 's/^toolchain //p' go.mod)"
export GOTOOLCHAIN="${pinned:-auto}"

go run ./tools/manifest rename "$@"
go mod edit -module "github.com/$(printf '%s' "$repo" | tr '[:upper:]' '[:lower:]')"
go generate ./...
go test -count=1 ./...

cat <<EOF

Renamed to "$name" (github.com/$repo). Next:
  1. filex-app.json: write your description and permission reasons,
     declare your actions and views.
  2. action.go / views.go / count.go: replace the word count with your app.
  3. README.md: it still describes the template.
  4. go test ./...  then  bash scripts/build.sh
EOF
