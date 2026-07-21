#!/bin/sh

set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "usage: $0 <tool> [amd64|arm64]" >&2
  exit 1
fi

PACKAGE_NAME="$1"
ARCH="${2:-$(dpkg --print-architecture)}"
GOARCH=""

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

case "${PACKAGE_NAME}" in
  *[!a-z0-9+.-]* | "")
    echo "invalid package name: ${PACKAGE_NAME}" >&2
    exit 1
    ;;
esac

if [ ! -d "cmd/${PACKAGE_NAME}" ]; then
  echo "missing command entrypoint: cmd/${PACKAGE_NAME}" >&2
  exit 1
fi

if [ ! -f "debian/${PACKAGE_NAME}/VERSION" ]; then
  echo "missing package version: debian/${PACKAGE_NAME}/VERSION" >&2
  exit 1
fi

if [ ! -f "debian/${PACKAGE_NAME}/control" ]; then
  echo "missing package control: debian/${PACKAGE_NAME}/control" >&2
  exit 1
fi

VERSION="$(cat "debian/${PACKAGE_NAME}/VERSION")"
case "${VERSION}" in
  *[!0-9.]* | "" | .* | *.)
    echo "invalid package version: ${VERSION}" >&2
    exit 1
    ;;
esac

OUTPUT_DIR="dist"
OUTPUT_DEB="${OUTPUT_DIR}/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
BUILD_ROOT="$(mktemp -d)"
PKG_ROOT="${BUILD_ROOT}/${PACKAGE_NAME}_${VERSION}_${ARCH}"

cleanup() {
  rm -rf "${BUILD_ROOT}"
}

trap cleanup EXIT INT TERM

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
    -trimpath \
    -buildvcs=false \
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

if [ "${PACKAGE_NAME}" = "xffuf" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xffuf.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xffuf"
  cp "debian/${PACKAGE_NAME}/xffuf.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xffuf"
fi

if [ "${PACKAGE_NAME}" = "xssh" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xssh.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xssh"
  cp "debian/${PACKAGE_NAME}/xssh.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xssh"
fi

if [ "${PACKAGE_NAME}" = "xwebshell" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xwebshell.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xwebshell"
  cp "debian/${PACKAGE_NAME}/xwebshell.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xwebshell"
fi

if [ "${PACKAGE_NAME}" = "xscp" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xscp.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xscp"
  cp "debian/${PACKAGE_NAME}/xscp.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xscp"
fi

if [ "${PACKAGE_NAME}" = "xhydra" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xhydra.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xhydra"
  cp "debian/${PACKAGE_NAME}/xhydra.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xhydra"
fi

if [ "${PACKAGE_NAME}" = "xmagic" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xmagic.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xmagic"
  cp "debian/${PACKAGE_NAME}/xmagic.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xmagic"
fi

if [ "${PACKAGE_NAME}" = "xsteg" ]; then
  mkdir -p "${PKG_ROOT}/usr/share/bash-completion/completions"
  mkdir -p "${PKG_ROOT}/usr/share/zsh/vendor-completions"
  cp "debian/${PACKAGE_NAME}/xsteg.bash" "${PKG_ROOT}/usr/share/bash-completion/completions/xsteg"
  cp "debian/${PACKAGE_NAME}/xsteg.zsh" "${PKG_ROOT}/usr/share/zsh/vendor-completions/_xsteg"
fi

dpkg-deb --root-owner-group --build "${PKG_ROOT}" "${OUTPUT_DEB}"

echo "created ${OUTPUT_DEB}"
