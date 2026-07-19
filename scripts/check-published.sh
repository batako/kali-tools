#!/bin/sh

set -u

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <tool>" >&2
  exit 1
fi

PACKAGE_NAME="$1"
VERSION_FILE="debian/${PACKAGE_NAME}/VERSION"
REPOSITORY_URL="${APT_REPOSITORY_URL:-https://offsec.batako.net}"
TMP_DIR="$(mktemp -d)"
FAILED=0

cleanup() {
  rm -rf "${TMP_DIR}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

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

fetch() {
  curl --fail --silent --show-error --location \
    --retry 5 --retry-delay 10 --retry-all-errors "$@"
}

check_packages_index() {
  arch="$1"
  packages_file="${TMP_DIR}/Packages-${arch}"

  fetch "${REPOSITORY_URL}/dists/stable/main/binary-${arch}/Packages.gz" |
    gzip -dc >"${packages_file}" &&
    awk -v package="${PACKAGE_NAME}" -v version="${VERSION}" '
      BEGIN { RS = ""; FS = "\n"; found = 0 }
      {
        package_match = 0
        version_match = 0
        for (i = 1; i <= NF; i++) {
          if ($i == "Package: " package) package_match = 1
          if ($i == "Version: " version) version_match = 1
        }
        if (package_match && version_match) found = 1
      }
      END { exit(found ? 0 : 1) }
    ' "${packages_file}"
}

check_published_deb() {
  arch="$1"
  prefix="$(printf '%s' "${PACKAGE_NAME}" | cut -c 1)"
  deb_file="${PACKAGE_NAME}_${VERSION}_${arch}.deb"
  deb_path="${TMP_DIR}/${deb_file}"
  deb_url="${REPOSITORY_URL}/pool/main/${prefix}/${PACKAGE_NAME}/${deb_file}"

  fetch --output "${deb_path}" "${deb_url}" &&
    test "$(dpkg-deb -f "${deb_path}" Package)" = "${PACKAGE_NAME}" &&
    test "$(dpkg-deb -f "${deb_path}" Version)" = "${VERSION}" &&
    test "$(dpkg-deb -f "${deb_path}" Architecture)" = "${arch}"
}

if [ ! -f "${VERSION_FILE}" ]; then
  printf 'NG   version file exists\n'
  printf '     missing %s\n' "${VERSION_FILE}"
  exit 1
fi

VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"

printf 'Post-release checks: %s %s\n' "${PACKAGE_NAME}" "${VERSION}"
printf 'Repository: %s\n\n' "${REPOSITORY_URL}"

for arch in amd64 arm64; do
  check "APT metadata contains ${PACKAGE_NAME} ${VERSION} (${arch})" check_packages_index "${arch}"
  check "published .deb is valid (${arch})" check_published_deb "${arch}"
done

printf '\nTODO:\n'
printf '%s\n' "- Run: sudo apt update && apt-cache policy ${PACKAGE_NAME}"
printf '%s\n' "- Run: sudo apt install ${PACKAGE_NAME}=${VERSION} && ${PACKAGE_NAME} -V"

printf '\n'
if [ "${FAILED}" -eq 0 ]; then
  printf 'Automated post-release checks passed. Complete the TODO items above.\n'
  exit 0
fi

printf 'Automated post-release checks failed. The release may still be propagating; retry later.\n'
exit 1
