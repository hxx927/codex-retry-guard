#!/usr/bin/env bash
set -euo pipefail

PLUGIN_ID="codex-retry-guard"
VERSION="${1:-0.1.0}"
export PLUGIN_ID VERSION
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
EXT="so"
case "$GOOS_VALUE" in
  darwin) EXT="dylib" ;;
  windows) EXT="dll" ;;
  linux|freebsd) EXT="so" ;;
  *) echo "unsupported GOOS: $GOOS_VALUE" >&2; exit 1 ;;
esac
export GOOS_VALUE GOARCH_VALUE EXT

rm -rf dist
mkdir -p dist/package
CGO_ENABLED=1 GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" go build -buildmode=c-shared -o "dist/package/${PLUGIN_ID}.${EXT}" ./cmd/plugin
python3 - <<PY
import os
import zipfile
plugin_id = os.environ.get("PLUGIN_ID", "codex-retry-guard")
version = os.environ["VERSION"]
goos = os.environ["GOOS_VALUE"]
goarch = os.environ["GOARCH_VALUE"]
ext = os.environ["EXT"]
zip_path = f"dist/{plugin_id}_{version}_{goos}_{goarch}.zip"
so_name = f"{plugin_id}.{ext}"
with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    zf.write(f"dist/package/{so_name}", so_name)
PY
(
  cd dist
  sha256sum "${PLUGIN_ID}_${VERSION}_${GOOS_VALUE}_${GOARCH_VALUE}.zip" > checksums.txt
)
rm -rf dist/package
