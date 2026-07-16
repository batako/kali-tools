#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <req|ctx|xssh|xscp|xftp|xsmb|xgobuster|xwebshell>" >&2
  exit 1
fi

PACKAGE_NAME="$1"
case "${PACKAGE_NAME}" in
  req|ctx|xssh|xscp|xftp|xsmb|xgobuster|xwebshell)
    ;;
  *)
    echo "unsupported package: ${PACKAGE_NAME}" >&2
    echo "usage: $0 <req|ctx|xssh|xscp|xftp|xsmb|xgobuster|xwebshell>" >&2
    exit 1
    ;;
esac

VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "debian/${PACKAGE_NAME}/VERSION")"
ARCH="$(dpkg --print-architecture)"
DEB_PATH="dist/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"

./scripts/build-deb.sh "${PACKAGE_NAME}" "${ARCH}"

ABSOLUTE_DEB_PATH="$(cd "$(dirname "${DEB_PATH}")" && pwd)/$(basename "${DEB_PATH}")"
sudo apt-get install --reinstall --allow-downgrades -y "${ABSOLUTE_DEB_PATH}"
