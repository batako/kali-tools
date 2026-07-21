#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <tool>" >&2
  exit 1
fi

PACKAGE_NAME="$1"
case "${PACKAGE_NAME}" in
  *[!a-z0-9+.-]* | "")
    echo "invalid package name: ${PACKAGE_NAME}" >&2
    exit 1
    ;;
esac

if { [ ! -d "cmd/${PACKAGE_NAME}" ] && [ ! -f "debian/${PACKAGE_NAME}/META_PACKAGE" ]; } ||
  [ ! -f "debian/${PACKAGE_NAME}/VERSION" ] ||
  [ ! -f "debian/${PACKAGE_NAME}/control" ]; then
  echo "missing package configuration for ${PACKAGE_NAME}" >&2
  exit 1
fi

VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "debian/${PACKAGE_NAME}/VERSION")"
ARCH="$(dpkg --print-architecture)"
if [ -f "debian/${PACKAGE_NAME}/META_PACKAGE" ]; then
  DEB_ARCH="all"
else
  DEB_ARCH="${ARCH}"
fi
DEB_PATH="dist/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}.deb"

./scripts/shared/build-deb.sh "${PACKAGE_NAME}" "${ARCH}"

ABSOLUTE_DEB_PATH="$(cd "$(dirname "${DEB_PATH}")" && pwd)/$(basename "${DEB_PATH}")"
sudo apt-get install --reinstall --allow-downgrades -y "${ABSOLUTE_DEB_PATH}"
