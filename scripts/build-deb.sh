#!/bin/sh

set -eu

PACKAGE_NAME="req"
VERSION="$(cat debian/req/VERSION)"
ARCH="$(dpkg --print-architecture)"
GOARCH=""
OUTPUT_DIR="dist"
OUTPUT_DEB="${OUTPUT_DIR}/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
BUILD_ROOT="$(mktemp -d)"
PKG_ROOT="${BUILD_ROOT}/${PACKAGE_NAME}_${VERSION}_${ARCH}"

cleanup() {
  rm -rf "${BUILD_ROOT}"
}

trap cleanup EXIT INT TERM

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command go
require_command dpkg-deb
require_command dpkg

case "${ARCH}" in
  amd64)
    GOARCH="amd64"
    ;;
  arm64)
    GOARCH="arm64"
    ;;
  *)
    echo "unsupported Debian architecture: ${ARCH}" >&2
    exit 1
    ;;
esac

mkdir -p "${PKG_ROOT}/DEBIAN"
mkdir -p "${PKG_ROOT}/usr/local/bin"
mkdir -p "${OUTPUT_DIR}"

sed \
  -e "s/@VERSION@/${VERSION}/" \
  -e "s/@ARCH@/${ARCH}/" \
  "debian/req/control" > "${PKG_ROOT}/DEBIAN/control"

CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" go build -o "${PKG_ROOT}/usr/local/bin/${PACKAGE_NAME}" ./cmd/req
chmod 0755 "${PKG_ROOT}/usr/local/bin/${PACKAGE_NAME}"

dpkg-deb --root-owner-group --build "${PKG_ROOT}" "${OUTPUT_DEB}"

echo "created ${OUTPUT_DEB}"
