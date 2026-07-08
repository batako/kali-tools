#!/bin/sh

set -eu

PACKAGE_NAME="${1:-ctx}"
VERSION_FILE="debian/${PACKAGE_NAME}/VERSION"
GO_FILE="internal/${PACKAGE_NAME}/run.go"

if [ ! -f "${VERSION_FILE}" ]; then
  echo "missing version file: ${VERSION_FILE}" >&2
  exit 1
fi

if [ ! -f "${GO_FILE}" ]; then
  echo "missing Go version file: ${GO_FILE}" >&2
  exit 1
fi

package_version="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"
go_version="$(sed -n 's/^[[:space:]]*Version[[:space:]]*=[[:space:]]*"\([^"]*\)"[[:space:]]*$/\1/p' "${GO_FILE}" | sed -n '1p')"

if [ -z "${package_version}" ]; then
  echo "empty package version: ${VERSION_FILE}" >&2
  exit 1
fi

if [ -z "${go_version}" ]; then
  echo "missing Version variable in ${GO_FILE}" >&2
  exit 1
fi

if [ "${package_version}" != "${go_version}" ]; then
  echo "version mismatch for ${PACKAGE_NAME}:" >&2
  echo "  ${VERSION_FILE}: ${package_version}" >&2
  echo "  ${GO_FILE}: ${go_version}" >&2
  exit 1
fi

echo "${PACKAGE_NAME} version OK: ${package_version}"
