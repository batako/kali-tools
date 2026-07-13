#!/bin/sh

set -u

RUN_BUNDLED_ADDONS=0
if [ "$#" -eq 0 ]; then
  RUN_BUNDLED_ADDONS=1
fi

PACKAGE_NAME="${1:-ctx}"
VERSION_FILE="debian/${PACKAGE_NAME}/VERSION"
ARCH="$(dpkg --print-architecture 2>/dev/null || true)"
TMP_DIR="$(mktemp -d)"
FAILED=0
RELEASE_CHECK_COMPLETED=0
INIT_SHELL_CHANGED=0
INIT_SHELL_RC_FILE=""
INIT_SHELL_BACKUP=""
INIT_SHELL_EXISTED=0
ACTIVE_SHELL_PATH=""

cleanup() {
  if [ "${INIT_SHELL_CHANGED}" -eq 1 ] && [ "${RELEASE_CHECK_COMPLETED}" -ne 1 ]; then
    if [ "${INIT_SHELL_EXISTED}" -eq 1 ]; then
      cp -p "${INIT_SHELL_BACKUP}" "${INIT_SHELL_RC_FILE}"
    else
      rm -f "${INIT_SHELL_RC_FILE}"
    fi
  fi
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
    return 1
  fi
}

check_package_files() {
  test -f "${VERSION_FILE}" &&
    test -f "debian/${PACKAGE_NAME}/control" &&
    test -d "cmd/${PACKAGE_NAME}"
}

check_tidy() {
  go mod tidy -diff
}

check_deb_metadata() {
  deb_path="$1"
  expected_version="$2"

  test "$(dpkg-deb -f "${deb_path}" Package)" = "${PACKAGE_NAME}" &&
    test "$(dpkg-deb -f "${deb_path}" Version)" = "${expected_version}" &&
    test "$(dpkg-deb -f "${deb_path}" Architecture)" = "${ARCH}"
}

check_deb_contents() {
  deb_path="$1"
  contents="${TMP_DIR}/deb-contents"
  dpkg-deb -c "${deb_path}" >"${contents}"
  grep -q "./usr/local/bin/${PACKAGE_NAME}$" "${contents}" || return 1
  if [ "${PACKAGE_NAME}" = "ctx" ]; then
    ! grep -q "./usr/local/bin/x$" "${contents}" || return 1
  fi
}

check_deb_version() {
  deb_path="$1"
  expected_version="$2"
  extract_dir="${TMP_DIR}/extract"

  rm -rf "${extract_dir}"
  mkdir -p "${extract_dir}"
  dpkg-deb -x "${deb_path}" "${extract_dir}"
  test "$("${extract_dir}/usr/local/bin/${PACKAGE_NAME}" -V)" = "${PACKAGE_NAME} ${expected_version}" || return 1
  if [ "${PACKAGE_NAME}" = "ctx" ]; then
    "${extract_dir}/usr/local/bin/ctx" x --help | grep -q "usage: ctx x <command>" || return 1
    "${extract_dir}/usr/local/bin/ctx" completion bash | grep -q 'x() { ctx x "$@"; }' || return 1
    "${extract_dir}/usr/local/bin/ctx" completion zsh | grep -q 'x() { ctx x "$@" }' || return 1
  fi
}

check_ctx_completion() {
  binary="$(command -v ctx)"
  "${binary}" completion bash | bash -n
  "${binary}" completion zsh | zsh -n
}

check_ctx_init_shell() {
  binary="$(command -v ctx)"
  shell_name="$(basename "${SHELL:-}")"
  real_home="${HOME:-}"

  case "${shell_name}" in
    zsh|bash) ;;
    *)
      shell_name="$(ps -p "${PPID}" -o comm= 2>/dev/null | sed 's/^[[:space:]-]*//;s/[[:space:]]*$//' | xargs basename 2>/dev/null || true)"
      ;;
  esac
  case "${shell_name}" in
    zsh|bash) ;;
    *)
      echo "SHELL must be zsh or bash for the init-shell check."
      return 1
      ;;
  esac
  shell_path="$(command -v "${shell_name}")"
  if [ -z "${real_home}" ]; then
    echo "HOME is required for the init-shell check."
    return 1
  fi

  rc_file="${real_home}/.${shell_name}rc"
  backup="${TMP_DIR}/${shell_name}rc-backup"
  existed=0
  completed=0

  if [ -e "${rc_file}" ]; then
    cp -p "${rc_file}" "${backup}"
    existed=1
  fi

  (
    restore_rc_file() {
      if [ "${completed}" -ne 1 ]; then
        if [ "${existed}" -eq 1 ]; then
          cp -p "${backup}" "${rc_file}"
        else
          rm -f "${rc_file}"
        fi
      fi
    }
    trap restore_rc_file EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    HOME="${real_home}" SHELL="${shell_path}" "${binary}" init-shell --remove >/dev/null
    ! grep -q '^# >>> ctx >>>$' "${rc_file}" 2>/dev/null

    before_mtime="$(stat -c %Y "${rc_file}" 2>/dev/null || printf '0')"
    sleep 1
    HOME="${real_home}" SHELL="${shell_path}" "${binary}" init-shell >/dev/null
    after_mtime="$(stat -c %Y "${rc_file}")"

    test "${after_mtime}" -gt "${before_mtime}" &&
      test "$(grep -c '^# >>> ctx >>>$' "${rc_file}")" -eq 1 &&
      grep -q "^source <(ctx completion ${shell_name})$" "${rc_file}" &&
      test "$(grep -c '^# <<< ctx <<<$' "${rc_file}")" -eq 1 ||
      exit 1

    cp "${rc_file}" "${TMP_DIR}/rc-snapshot"
    before_mtime="${after_mtime}"
    sleep 1
    HOME="${real_home}" SHELL="${shell_path}" "${binary}" init-shell >/dev/null
    after_mtime="$(stat -c %Y "${rc_file}")"

    test "${after_mtime}" = "${before_mtime}" &&
      cmp -s "${rc_file}" "${TMP_DIR}/rc-snapshot" ||
      exit 1

    sed "s/source <(ctx completion ${shell_name})/source <(ctx completion old-shell)/" \
      "${rc_file}" >"${TMP_DIR}/rc-stale"
    cat "${TMP_DIR}/rc-stale" >"${rc_file}"
    before_mtime="$(stat -c %Y "${rc_file}")"
    sleep 1
    HOME="${real_home}" SHELL="${shell_path}" "${binary}" init-shell >/dev/null
    after_mtime="$(stat -c %Y "${rc_file}")"

    test "${after_mtime}" -gt "${before_mtime}" &&
      grep -q "^source <(ctx completion ${shell_name})$" "${rc_file}" &&
      ! grep -q 'old-shell' "${rc_file}" ||
      exit 1

    HOME="${real_home}" "${shell_path}" -ic \
      'type x >/dev/null && type xtarget >/dev/null && type xhosts >/dev/null && ctx -V >/dev/null && ctx x --help >/dev/null && x 2>&1 | grep -q "usage: ctx x <command>"'

    completed=1
  )
  result=$?
  if [ "${result}" -ne 0 ]; then
    return "${result}"
  fi

  INIT_SHELL_CHANGED=1
  INIT_SHELL_RC_FILE="${rc_file}"
  INIT_SHELL_BACKUP="${backup}"
  INIT_SHELL_EXISTED="${existed}"
  ACTIVE_SHELL_PATH="${shell_path}"
}

check_deb_install() {
  deb_path="$1"
  expected_version="$2"
  absolute_deb_path="$(cd "$(dirname "${deb_path}")" && pwd)/$(basename "${deb_path}")"
  install_output="${TMP_DIR}/apt-install-output"

  if ! sudo apt-get install --reinstall --allow-downgrades -y "${absolute_deb_path}" >"${install_output}" 2>&1 ||
    ! test "$(dpkg-query -W -f='${Version}' "${PACKAGE_NAME}")" = "${expected_version}" ||
    ! test "$("${PACKAGE_NAME}" -V)" = "${PACKAGE_NAME} ${expected_version}" ||
    ! "${PACKAGE_NAME}" --help >/dev/null; then
    cat "${install_output}"
    return 1
  fi

  if [ "${PACKAGE_NAME}" = "ctx" ]; then
    if ! grep -q "ctx installed successfully." "${install_output}" ||
      ! grep -q "ctx init-shell" "${install_output}" ||
      ! ctx x --help | grep -q "usage: ctx x <command>" ||
      ! ctx completion bash | grep -q 'x() { ctx x "$@"; }' ||
      ! ctx completion zsh | grep -q 'x() { ctx x "$@" }' ||
      ! SHELL=/usr/bin/zsh ctx doctor 2>&1 | grep -A1 '^OK  Shell$' | grep -q 'zsh'; then
      cat "${install_output}"
      return 1
    fi
  fi

  if ! sudo apt-get remove -y "${PACKAGE_NAME}" >/dev/null ||
    ! test ! -e "/usr/local/bin/${PACKAGE_NAME}" ||
    ! sudo apt-get install --allow-downgrades -y "${absolute_deb_path}" >/dev/null; then
    return 1
  fi
}

check_addon_package() {
  addon_name="$1"
  old_package_name="${PACKAGE_NAME}"
  old_version_file="${VERSION_FILE}"
  old_version="${VERSION}"

  PACKAGE_NAME="${addon_name}"
  VERSION_FILE="debian/${PACKAGE_NAME}/VERSION"

  if [ -f "${VERSION_FILE}" ]; then
    VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"
  else
    VERSION=""
  fi

  check "${PACKAGE_NAME} release files exist" check_package_files
  check "${PACKAGE_NAME} source and package versions match" ./scripts/check-version.sh "${PACKAGE_NAME}"

  if [ -n "${ARCH}" ] && [ -n "${VERSION}" ]; then
    deb_path="dist/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
    check "${PACKAGE_NAME} Debian package builds (${ARCH})" ./scripts/build-deb.sh "${PACKAGE_NAME}" "${ARCH}"
    check "${PACKAGE_NAME} Debian package metadata is correct" check_deb_metadata "${deb_path}" "${VERSION}"
    check "${PACKAGE_NAME} Debian package contains the executable" check_deb_contents "${deb_path}"
    check "${PACKAGE_NAME} packaged executable reports ${VERSION}" check_deb_version "${deb_path}" "${VERSION}"
    check "${PACKAGE_NAME} APT install, removal, and reinstall work on Kali Linux" check_deb_install "${deb_path}" "${VERSION}"
  else
    printf 'NG   %s package version and architecture are available\n' "${PACKAGE_NAME}"
    FAILED=1
  fi

  PACKAGE_NAME="${old_package_name}"
  VERSION_FILE="${old_version_file}"
  VERSION="${old_version}"
}

printf 'Pre-release checks: %s\n\n' "${PACKAGE_NAME}"

check "release files exist" check_package_files

if [ -f "${VERSION_FILE}" ]; then
  VERSION="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${VERSION_FILE}")"
else
  VERSION=""
fi

check "Go modules are tidy" check_tidy
check "Go tests pass" go test ./...

if [ "${PACKAGE_NAME}" = "ctx" ] || [ "${PACKAGE_NAME}" = "xssh" ] || [ "${PACKAGE_NAME}" = "xftp" ] || [ "${PACKAGE_NAME}" = "xsmb" ]; then
  check "source and package versions match" ./scripts/check-version.sh "${PACKAGE_NAME}"
fi

if [ -n "${ARCH}" ] && [ -n "${VERSION}" ]; then
  DEB_PATH="dist/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"
  check "Debian package builds (${ARCH})" ./scripts/build-deb.sh "${PACKAGE_NAME}" "${ARCH}"
  check "Debian package metadata is correct" check_deb_metadata "${DEB_PATH}" "${VERSION}"
  check "Debian package contains the executable" check_deb_contents "${DEB_PATH}"
  check "packaged executable reports ${VERSION}" check_deb_version "${DEB_PATH}" "${VERSION}"

  printf 'INFO Administrator privileges are required for the APT install check.\n'
  if check "APT install, removal, and reinstall work on Kali Linux" check_deb_install "${DEB_PATH}" "${VERSION}"; then
    if [ "${PACKAGE_NAME}" = "ctx" ]; then
      check "installed ctx prints valid bash and zsh completion scripts" check_ctx_completion
      check "installed ctx configures and loads the current shell" check_ctx_init_shell
    fi
  elif [ "${PACKAGE_NAME}" = "ctx" ]; then
    printf 'NG   installed ctx shell checks\n'
    printf '     skipped because the APT install check failed\n'
  fi
else
  printf 'NG   package version and architecture are available\n'
  FAILED=1
fi

if [ "${PACKAGE_NAME}" = "ctx" ] && [ "${RUN_BUNDLED_ADDONS}" -eq 1 ]; then
  check_addon_package xssh
  check_addon_package xftp
  check_addon_package xsmb
fi

if [ "${PACKAGE_NAME}" = "ctx" ] && [ "${FAILED}" -eq 0 ]; then
  printf '\nTODO:\n'
  printf '%s\n' '- In the loaded shell, confirm ctx and x-prefixed Tab completion'
  printf '%s\n' '- Run ctx hosts sync and confirm only that operation requests sudo'
fi

printf '\n'
if [ "${FAILED}" -eq 0 ]; then
  RELEASE_CHECK_COMPLETED=1
  printf 'Automated pre-release checks passed. Complete the TODO items above.\n'
  if [ "${PACKAGE_NAME}" = "ctx" ] && [ -n "${ACTIVE_SHELL_PATH}" ] && [ -t 0 ] && [ -t 1 ]; then
    printf 'Starting %s with the updated ctx shell configuration. Exit it with Ctrl-D.\n' "${ACTIVE_SHELL_PATH}"
    "${ACTIVE_SHELL_PATH}" -i
  fi
  exit 0
fi

printf 'Pre-release checks failed.\n'
exit 1
