#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <tool>" >&2
  exit 1
fi

PACKAGE_NAME="$1"
VERSION_FILE="debian/${PACKAGE_NAME}/VERSION"
GO_DIR="internal/${PACKAGE_NAME}"

if [ -f "debian/${PACKAGE_NAME}/META_PACKAGE" ]; then
  echo "${PACKAGE_NAME} is a meta-package; no Go version check required"
  exit 0
fi

if [ ! -f "${VERSION_FILE}" ]; then
  echo "missing version file: ${VERSION_FILE}" >&2
  exit 1
fi

if [ ! -d "${GO_DIR}" ]; then
  echo "missing Go package directory: ${GO_DIR}" >&2
  exit 1
fi

package_version="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"
go_version="$(
  sed -n 's/^[[:space:]]*\(var[[:space:]]*\)\{0,1\}Version[[:space:]]*=[[:space:]]*"\([^"]*\)"[[:space:]]*$/\2/p' \
    "${GO_DIR}"/*.go | sed -n '1p'
)"

if [ -z "${package_version}" ]; then
  echo "empty package version: ${VERSION_FILE}" >&2
  exit 1
fi

if [ -z "${go_version}" ]; then
  echo "missing Version variable in ${GO_DIR}" >&2
  exit 1
fi

if [ "${package_version}" != "${go_version}" ]; then
  echo "version mismatch for ${PACKAGE_NAME}:" >&2
  echo "  ${VERSION_FILE}: ${package_version}" >&2
  echo "  ${GO_DIR}: ${go_version}" >&2
  exit 1
fi

echo "${PACKAGE_NAME} version OK: ${package_version}"
