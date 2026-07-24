#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${VERSION:-0.0.0-local.$(git -C "$ROOT" rev-parse --short HEAD)}"
ARCH="${ARCH:-$(dpkg --print-architecture)}"
STAGE="${STAGE:-/tmp/retri-local-deb}"
OUT_DIR="${OUT_DIR:-/tmp/retri-artifacts}"
OUT="${OUT:-$OUT_DIR/retri_${VERSION}_${ARCH}.deb}"

case "$STAGE" in
  /tmp/retri-local-deb | /tmp/retri-local-deb/*) ;;
  *)
    echo "Refusing to remove unsafe STAGE path: $STAGE" >&2
    exit 1
    ;;
esac

case "$OUT_DIR" in
  /tmp/retri-artifacts | /tmp/retri-artifacts/*) ;;
  *)
    echo "Refusing to write outside /tmp/retri-artifacts: $OUT_DIR" >&2
    exit 1
    ;;
esac

case "$OUT" in
  "$OUT_DIR"/*) ;;
  *)
    echo "Refusing to remove output outside OUT_DIR: $OUT" >&2
    exit 1
    ;;
esac

rm -rf "$STAGE" "$OUT"
mkdir -p \
  "$STAGE/DEBIAN" \
  "$STAGE/usr/bin" \
  "$STAGE/usr/share/bash-completion/completions" \
  "$STAGE/usr/share/zsh/vendor-completions" \
  "$STAGE/usr/share/fish/vendor_completions.d" \
  "$OUT_DIR"

GOCACHE="${GOCACHE:-/tmp/retri-gocache}" go build \
  -C "$ROOT" \
  -o "$STAGE/usr/bin/retri" \
  -ldflags "-s -w -X main.Version=${VERSION}" \
  .

cp "$ROOT/completions/retri.bash" "$STAGE/usr/share/bash-completion/completions/retri"
cp "$ROOT/completions/retri.zsh" "$STAGE/usr/share/zsh/vendor-completions/_retri"
cp "$ROOT/completions/retri.fish" "$STAGE/usr/share/fish/vendor_completions.d/retri.fish"
cp "$ROOT/packaging/deb/postinst" "$STAGE/DEBIAN/postinst"
chmod 0755 "$STAGE/usr/bin/retri"
chmod 0755 "$STAGE/DEBIAN/postinst"

cat > "$STAGE/DEBIAN/control" <<EOF
Package: retri
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Maintainer: cotta-dev
Homepage: https://github.com/cotta-dev/retri
Description: Universal SSH Log Collector & Command Executor
EOF

dpkg-deb --root-owner-group --build "$STAGE" "$OUT"
echo "$OUT"
