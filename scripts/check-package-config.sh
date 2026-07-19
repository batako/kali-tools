#!/bin/sh

set -u

FAILED=0
PACKAGES=0

ok() {
  printf 'OK   %s\n' "$1"
}

ng() {
  printf 'NG   %s\n' "$1"
  FAILED=1
}

check_file() {
  label="$1"
  path="$2"

  if [ -f "${path}" ]; then
    ok "${label}"
  else
    ng "${label}: missing ${path}"
  fi
}

check_control() {
  package="$1"
  control="debian/${package}/control"

  if [ ! -f "${control}" ]; then
    return
  fi

  if grep -Fx "Package: ${package}" "${control}" >/dev/null; then
    ok "Debian package name"
  else
    ng "Debian package name: expected Package: ${package}"
  fi

  if grep -Fx 'Version: @VERSION@' "${control}" >/dev/null; then
    ok "Debian version placeholder"
  else
    ng "Debian version placeholder: missing Version: @VERSION@"
  fi

  if grep -Fx 'Architecture: @ARCH@' "${control}" >/dev/null; then
    ok "Debian architecture placeholder"
  else
    ng "Debian architecture placeholder: missing Architecture: @ARCH@"
  fi
}

uses_ctx() {
  package="$1"

  if [ "${package}" = "ctx" ]; then
    return 1
  fi

  find "cmd/${package}" "internal/${package}" \
    -type f -name '*.go' ! -name '*_test.go' \
    -exec grep -Eq '"req/internal/(ctx|ctxapi|ctxexec)"' {} +
}

check_ctx_dependency() {
  package="$1"
  control="debian/${package}/control"

  if ! uses_ctx "${package}"; then
    return
  fi

  if [ ! -f "${control}" ]; then
    return
  fi

  if grep -Eq '^Depends:([^,]*,)*[[:space:]]*ctx([[:space:]]*\([^)]*\))?([[:space:]]*,|[[:space:]]*$)' "${control}"; then
    ok "ctx Debian dependency"
  else
    ng "ctx Debian dependency: ${control} must include ctx"
  fi
}

for required_file in \
  scripts/build-deb.sh \
  scripts/install-deb.sh \
  scripts/check-version.sh \
  scripts/check-release.sh \
  .github/workflows/test.yml \
  .github/workflows/release.yml \
  .github/workflows/audit-releases.yml; do
  check_file "required project file" "${required_file}"
done

for command_dir in cmd/*; do
  if [ ! -d "${command_dir}" ]; then
    continue
  fi

  package="$(basename "${command_dir}")"
  package_dir="debian/${package}"
  PACKAGES=$((PACKAGES + 1))

  printf '\nPackage configuration: %s\n' "${package}"

  if [ -d "${package_dir}" ]; then
    ok "Debian package directory"
  else
    ng "Debian package directory: missing ${package_dir}"
  fi

  check_file "command entrypoint" "cmd/${package}/main.go"
  if [ -d "internal/${package}" ]; then
    ok "Go package directory"
  else
    ng "Go package directory: missing internal/${package}"
  fi
  check_file "Debian version" "${package_dir}/VERSION"
  check_file "Debian control" "${package_dir}/control"
  check_control "${package}"
  check_ctx_dependency "${package}"

  if [ -f "${package_dir}/VERSION" ]; then
    version="$(sed -n '1{s/[[:space:]]//g;p;q;}' "${package_dir}/VERSION")"
    check_file "English release notes" "releases/${package}/${version}.md"
    check_file "Japanese release notes" "releases/${package}/${version}.ja.md"
  fi
done

for package_dir in debian/*; do
  if [ ! -d "${package_dir}" ]; then
    continue
  fi

  package="$(basename "${package_dir}")"
  if [ ! -d "cmd/${package}" ]; then
    ng "orphan Debian package: missing cmd/${package}"
  fi
done

printf '\n'
if [ "${PACKAGES}" -eq 0 ]; then
  printf 'NG   no command directories found\n' >&2
  exit 1
fi

if [ "${FAILED}" -ne 0 ]; then
  printf 'Package configuration check failed.\n' >&2
  exit 1
fi

printf 'Package configuration check passed for %s package(s).\n' "${PACKAGES}"
