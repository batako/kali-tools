#!/bin/sh

set -eu

REPO_ROOT="repo"
POOL_ROOT="${REPO_ROOT}/pool/main"
DISTS_MAIN_DIR="${REPO_ROOT}/dists/stable/main"
BINARY_ALL_DIR="${DISTS_MAIN_DIR}/binary-all"
ARCHES_FILE="$(mktemp)"

cleanup() {
  rm -f "${ARCHES_FILE}"
}

trap cleanup EXIT INT TERM

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command cut
require_command dpkg-scanpackages
require_command gzip
require_command sort

mkdir -p "${POOL_ROOT}"
mkdir -p "${DISTS_MAIN_DIR}"

for deb_path in dist/*.deb; do
  if [ ! -e "${deb_path}" ]; then
    continue
  fi

  deb_file="$(basename "${deb_path}")"
  package_name="${deb_file%%_*}"
  package_prefix="$(printf '%s' "${package_name}" | cut -c 1)"
  pool_dir="${POOL_ROOT}/${package_prefix}/${package_name}"

  mkdir -p "${pool_dir}"
  cp "${deb_path}" "${pool_dir}/"
done

find "${POOL_ROOT}" -type f -name '*.deb' | while IFS= read -r deb_path; do
  deb_file="$(basename "${deb_path}")"
  arch="${deb_file##*_}"
  arch="${arch%.deb}"
  printf '%s\n' "${arch}" >> "${ARCHES_FILE}"
done

if [ ! -s "${ARCHES_FILE}" ]; then
  echo "missing package files in ${POOL_ROOT}" >&2
  exit 1
fi

find "${DISTS_MAIN_DIR}" -mindepth 1 -maxdepth 1 -type d -name 'binary-*' -exec rm -rf {} +
mkdir -p "${BINARY_ALL_DIR}"
: > "${BINARY_ALL_DIR}/Packages"
gzip -kf "${BINARY_ALL_DIR}/Packages"

sort -u "${ARCHES_FILE}" | while IFS= read -r arch; do
  if [ "${arch}" = "all" ]; then
    continue
  fi

  binary_dir="${REPO_ROOT}/dists/stable/main/binary-${arch}"
  mkdir -p "${binary_dir}"

  (
    cd "${REPO_ROOT}"
    dpkg-scanpackages --arch "${arch}" --multiversion pool > "dists/stable/main/binary-${arch}/Packages"
  )

  gzip -kf "${binary_dir}/Packages"
done

echo "updated ${REPO_ROOT}"
