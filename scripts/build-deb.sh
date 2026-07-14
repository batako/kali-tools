#!/bin/sh

set -eu

PACKAGE_NAME="req"

if [ "$#" -gt 0 ]; then
  case "$1" in
    req|ctx|xssh|xftp|xsmb|xgobuster)
      PACKAGE_NAME="$1"
      shift
      ;;
  esac
fi

ARCH="${1:-$(dpkg --print-architecture)}"
VERSION="$(cat "debian/${PACKAGE_NAME}/VERSION")"
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

if [ ! -d "cmd/${PACKAGE_NAME}" ]; then
  echo "missing command entrypoint: cmd/${PACKAGE_NAME}" >&2
  exit 1
fi

mkdir -p "${PKG_ROOT}/DEBIAN"
mkdir -p "${PKG_ROOT}/usr/local/bin"
mkdir -p "${OUTPUT_DIR}"

sed \
  -e "s/@VERSION@/${VERSION}/" \
  -e "s/@ARCH@/${ARCH}/" \
  "debian/${PACKAGE_NAME}/control" > "${PKG_ROOT}/DEBIAN/control"

for script in postinst prerm postrm preinst; do
  if [ -f "debian/${PACKAGE_NAME}/${script}" ]; then
    cp "debian/${PACKAGE_NAME}/${script}" "${PKG_ROOT}/DEBIAN/${script}"
    chmod 0755 "${PKG_ROOT}/DEBIAN/${script}"
  fi
done

build_binary() {
  binary_name="$1"
  package_path="$2"
  CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" go build \
    -ldflags "-X req/internal/${PACKAGE_NAME}.Version=${VERSION}" \
    -o "${PKG_ROOT}/usr/local/bin/${binary_name}" \
    "${package_path}"
  chmod 0755 "${PKG_ROOT}/usr/local/bin/${binary_name}"
}

build_binary "${PACKAGE_NAME}" "./cmd/${PACKAGE_NAME}"

if [ "${PACKAGE_NAME}" = "xgobuster" ]; then
  ln -s "${PACKAGE_NAME}" "${PKG_ROOT}/usr/local/bin/xgo"
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xgobuster.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xgobuster"
  cp "debian/${PACKAGE_NAME}/xgobuster.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xgobuster"
  cp "debian/${PACKAGE_NAME}/xgo.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xgo"
fi

dpkg-deb --root-owner-group --build "${PKG_ROOT}" "${OUTPUT_DEB}"

echo "created ${OUTPUT_DEB}"
