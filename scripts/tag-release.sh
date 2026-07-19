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

VERSION_FILE="debian/${PACKAGE_NAME}/VERSION"

if [ "$(git branch --show-current)" != "main" ]; then
  echo "release tags must be created from main" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "working tree must be clean before creating a release tag" >&2
  exit 1
fi

if [ ! -f "${VERSION_FILE}" ]; then
  echo "missing package version: ${VERSION_FILE}" >&2
  exit 1
fi

if git show-ref --verify --quiet refs/remotes/origin/main &&
  [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
  echo "main must match origin/main before creating a release tag" >&2
  exit 1
fi

VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"
case "${VERSION}" in
  *[!0-9.]* | "" | .* | *.)
    echo "invalid package version: ${VERSION}" >&2
    exit 1
    ;;
esac

TAG="${PACKAGE_NAME}/v${VERSION}"

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "release tag already exists: ${TAG}" >&2
  exit 1
fi

./scripts/check-release.sh "${PACKAGE_NAME}"

git tag -a "${TAG}" -m "${PACKAGE_NAME} v${VERSION}"

printf '\nCreated %s. Push it with:\n\n' "${TAG}"
printf '  git push origin %s\n' "${TAG}"
