#!/bin/sh

set -eu

REPO_ROOT="repo"
POOL_ROOT="${REPO_ROOT}/pool/main"
DISTS_MAIN_DIR="${REPO_ROOT}/dists/stable/main"
BINARY_ALL_DIR="${DISTS_MAIN_DIR}/binary-all"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command cut
require_command dpkg-scanpackages
require_command gzip

set -- dist/*.deb
if [ ! -e "$1" ]; then
  echo "missing package files: dist/*.deb" >&2
  exit 1
fi

mkdir -p "${POOL_ROOT}"
mkdir -p "${DISTS_MAIN_DIR}"
find "${POOL_ROOT}" -type f -name '*.deb' -delete
find "${DISTS_MAIN_DIR}" -mindepth 1 -maxdepth 1 -type d -name 'binary-*' -exec rm -rf {} +
mkdir -p "${BINARY_ALL_DIR}"
: > "${BINARY_ALL_DIR}/Packages"
gzip -kf "${BINARY_ALL_DIR}/Packages"

for deb_path in dist/*.deb; do
  deb_file="$(basename "${deb_path}")"
  package_name="${deb_file%%_*}"
  package_prefix="$(printf '%s' "${package_name}" | cut -c 1)"
  arch="${deb_file##*_}"
  arch="${arch%.deb}"
  pool_dir="${POOL_ROOT}/${package_prefix}/${package_name}"
  binary_dir="${REPO_ROOT}/dists/stable/main/binary-${arch}"

  mkdir -p "${pool_dir}"
  mkdir -p "${binary_dir}"
  cp "${deb_path}" "${pool_dir}/"
done

for binary_dir in "${DISTS_MAIN_DIR}"/binary-*; do
  arch="${binary_dir##*-}"
  if [ "${arch}" = "all" ]; then
    continue
  fi

  (
    cd "${REPO_ROOT}"
    dpkg-scanpackages --arch "${arch}" pool > "dists/stable/main/binary-${arch}/Packages"
  )

  gzip -kf "${binary_dir}/Packages"
done

echo "updated ${REPO_ROOT}"
