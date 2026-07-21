#!/bin/sh

set -u

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
FAILED=0
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "${TMP_DIR}"
}

trap cleanup EXIT INT TERM

check() {
  label="$1"
  shift
  output="${TMP_DIR}/check-output"

  if "$@" >"${output}" 2>&1; then
    printf 'OK   %s\n' "${label}"
  else
    printf 'NG   %s\n' "${label}"
    sed 's/^/     /' "${output}"
    FAILED=1
  fi
}

check_release_files() {
  if [ -f "debian/${PACKAGE_NAME}/META_PACKAGE" ]; then
    test -f "debian/${PACKAGE_NAME}/control"
  else
    test -d "cmd/${PACKAGE_NAME}" &&
      test -d "internal/${PACKAGE_NAME}" &&
      test -f "debian/${PACKAGE_NAME}/control"
  fi &&
    test -s "releases/${PACKAGE_NAME}/${VERSION}.md" &&
    test -s "releases/${PACKAGE_NAME}/${VERSION}.ja.md"
}

check_commands() {
  for command_name in go gofmt dpkg dpkg-deb git; do
    if ! command -v "${command_name}" >/dev/null 2>&1; then
      echo "missing required command: ${command_name}" >&2
      return 1
    fi
  done
}

check_format() {
  files="$(gofmt -l cmd internal)"
  if [ -n "${files}" ]; then
    printf '%s\n' "${files}"
    return 1
  fi
}

check_deb() {
  arch="$1"
  if [ -f "debian/${PACKAGE_NAME}/META_PACKAGE" ]; then
    deb_arch="all"
  else
    deb_arch="${arch}"
  fi
  deb_path="dist/${PACKAGE_NAME}_${VERSION}_${deb_arch}.deb"
  contents="${TMP_DIR}/contents-${arch}"
  extract_dir="${TMP_DIR}/extract-${arch}"

  if ! test -f "${deb_path}" ||
    ! test "$(dpkg-deb -f "${deb_path}" Package)" = "${PACKAGE_NAME}" ||
    ! test "$(dpkg-deb -f "${deb_path}" Version)" = "${VERSION}" ||
    ! test "$(dpkg-deb -f "${deb_path}" Architecture)" = "${deb_arch}" ||
    ! dpkg-deb -c "${deb_path}" >"${contents}"; then
    return 1
  fi

  if [ ! -f "debian/${PACKAGE_NAME}/META_PACKAGE" ] &&
    ! grep -q "./usr/local/bin/${PACKAGE_NAME}$" "${contents}"; then
    return 1
  fi

  if [ "$(dpkg --print-architecture)" = "${arch}" ]; then
    mkdir -p "${extract_dir}"
    dpkg-deb -x "${deb_path}" "${extract_dir}" &&
      test "$("${extract_dir}/usr/local/bin/${PACKAGE_NAME}" -V)" = "${PACKAGE_NAME} ${VERSION}"
  fi
}

check_tag() {
  tag="${PACKAGE_NAME}/v${VERSION}"

  if ! git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
    return 0
  fi

  test "$(git rev-list -n 1 "${tag}")" = "$(git rev-parse HEAD)"
}

if [ ! -f "${VERSION_FILE}" ]; then
  echo "missing package version: ${VERSION_FILE}" >&2
  exit 1
fi

VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"
case "${VERSION}" in
  *[!0-9.]* | "" | .* | *.)
    echo "invalid package version: ${VERSION}" >&2
    exit 1
    ;;
esac

printf 'Pre-release checks: %s %s\n\n' "${PACKAGE_NAME}" "${VERSION}"

check "required commands are available" check_commands
check "package configuration is complete" ./scripts/check-package-config.sh
check "release files exist" check_release_files
check "source and package versions match" ./scripts/check-version.sh "${PACKAGE_NAME}"
check "Go files are formatted" check_format
check "Go modules are tidy" go mod tidy -diff
check "Go tests pass" go test ./...
check "release tag is absent or points to HEAD" check_tag

if [ -f "debian/${PACKAGE_NAME}/META_PACKAGE" ]; then
  check "Debian package builds (all)" ./scripts/build-deb.sh "${PACKAGE_NAME}" amd64
  check "Debian package is valid (all)" check_deb all
else
  for arch in amd64 arm64; do
    check "Debian package builds (${arch})" ./scripts/build-deb.sh "${PACKAGE_NAME}" "${arch}"
    check "Debian package is valid (${arch})" check_deb "${arch}"
  done
fi

printf '\n'
if [ "${FAILED}" -ne 0 ]; then
  printf 'Pre-release checks failed.\n' >&2
  exit 1
fi

printf 'Pre-release checks passed for %s %s.\n' "${PACKAGE_NAME}" "${VERSION}"
