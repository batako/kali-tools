#!/bin/sh

set -eu

PACKAGE_NAME="req"
VERSION="$(cat debian/req/VERSION)"
ARCH="$(dpkg --print-architecture)"
OUTPUT_DEB="dist/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
REPO_ROOT="repo"
POOL_DIR="${REPO_ROOT}/pool/main/r/${PACKAGE_NAME}"
BINARY_DIR="${REPO_ROOT}/dists/stable/main/binary-${ARCH}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command dpkg-scanpackages
require_command gzip

if [ ! -f "${OUTPUT_DEB}" ]; then
  echo "missing package file: ${OUTPUT_DEB}" >&2
  exit 1
fi

mkdir -p "${POOL_DIR}"
mkdir -p "${BINARY_DIR}"

find "${POOL_DIR}" -maxdepth 1 -type f -name "${PACKAGE_NAME}_*.deb" -delete
cp "${OUTPUT_DEB}" "${POOL_DIR}/"

(
  cd "${REPO_ROOT}"
  dpkg-scanpackages --arch "${ARCH}" pool > "dists/stable/main/binary-${ARCH}/Packages"
)

gzip -kf "${BINARY_DIR}/Packages"

echo "updated ${REPO_ROOT}"
