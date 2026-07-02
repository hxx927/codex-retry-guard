#!/usr/bin/env bash
set -euo pipefail

PLUGIN_ID="codex-retry-guard"
VERSION="${1:-0.1.0}"
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
EXT="so"
case "$GOOS_VALUE" in
  darwin) EXT="dylib" ;;
  windows) EXT="dll" ;;
  linux|freebsd) EXT="so" ;;
  *) echo "unsupported GOOS: $GOOS_VALUE" >&2; exit 1 ;;
esac

rm -rf dist
mkdir -p dist/package
CGO_ENABLED=1 GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" go build -buildmode=c-shared -o "dist/package/${PLUGIN_ID}.${EXT}" ./cmd/plugin
(
  cd dist/package
  zip -q "../${PLUGIN_ID}_${VERSION}_${GOOS_VALUE}_${GOARCH_VALUE}.zip" "${PLUGIN_ID}.${EXT}"
)
(
  cd dist
  sha256sum "${PLUGIN_ID}_${VERSION}_${GOOS_VALUE}_${GOARCH_VALUE}.zip" > checksums.txt
)
rm -rf dist/package
