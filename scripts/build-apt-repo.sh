#!/bin/sh

set -eu

PACKAGE_NAME="req"
VERSION="$(cat debian/req/VERSION)"
REPO_ROOT="repo"
POOL_DIR="${REPO_ROOT}/pool/main/r/${PACKAGE_NAME}"
DISTS_MAIN_DIR="${REPO_ROOT}/dists/stable/main"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command dpkg-scanpackages
require_command gzip

set -- dist/${PACKAGE_NAME}_${VERSION}_*.deb
if [ ! -e "$1" ]; then
  echo "missing package files: dist/${PACKAGE_NAME}_${VERSION}_*.deb" >&2
  exit 1
fi

mkdir -p "${POOL_DIR}"
mkdir -p "${DISTS_MAIN_DIR}"
find "${POOL_DIR}" -maxdepth 1 -type f -name "${PACKAGE_NAME}_*.deb" -delete
find "${DISTS_MAIN_DIR}" -mindepth 1 -maxdepth 1 -type d -name 'binary-*' -exec rm -rf {} +

for deb_path in dist/${PACKAGE_NAME}_${VERSION}_*.deb; do
  deb_file="$(basename "${deb_path}")"
  arch="${deb_file##*_}"
  arch="${arch%.deb}"
  binary_dir="${REPO_ROOT}/dists/stable/main/binary-${arch}"

  mkdir -p "${binary_dir}"
  cp "${deb_path}" "${POOL_DIR}/"

  (
    cd "${REPO_ROOT}"
    dpkg-scanpackages --arch "${arch}" pool > "dists/stable/main/binary-${arch}/Packages"
  )

  gzip -kf "${binary_dir}/Packages"
done

echo "updated ${REPO_ROOT}"
