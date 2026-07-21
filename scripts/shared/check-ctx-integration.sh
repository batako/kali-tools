#!/bin/sh

set -u

FAILED=0

ok() {
  printf 'OK   %s\n' "$1"
}

ng() {
  printf 'NG   %s\n' "$1"
  FAILED=1
}

check_file() {
  path="$1"
  if [ -f "${path}" ]; then
    ok "${path} exists"
  else
    ng "${path} is missing"
  fi
}

check_text() {
  path="$1"
  text="$2"
  label="$3"
  if grep -F "${text}" "${path}" >/dev/null 2>&1; then
    ok "${label}"
  else
    ng "${label}: missing in ${path}"
  fi
}

printf 'ctx integration audit\n\n'

for path in \
  docs/api.md \
  docs/api.ja.md \
  docs/integration.md \
  docs/integration.ja.md \
  docs/database.md \
  docs/database.ja.md \
  internal/ctxapi/client.go \
  internal/ctxapi/client_test.go \
  internal/ctxexec/ctxexec.go \
  internal/ctxexec/ctxexec_test.go; do
  check_file "${path}"
done

check_text docs/api.md '## Common Response' 'English JSON envelope contract'
check_text docs/api.ja.md '## 共通レスポンス' 'Japanese JSON envelope contract'
check_text docs/api.md '## Exit Codes' 'English exit-code contract'
check_text docs/api.ja.md '## 終了コード' 'Japanese exit-code contract'
check_text docs/api.md '## `formats`' 'English capability discovery contract'
check_text docs/api.ja.md '## `formats`' 'Japanese capability discovery contract'
check_text docs/integration.md '## Language-Independent Integration Procedure' 'English language-independent procedure'
check_text docs/integration.ja.md '## 言語非依存の連携手順' 'Japanese language-independent procedure'
check_text docs/integration.md '### Starting ctx safely' 'English fixed-path guidance'
check_text docs/integration.ja.md '### ctxを安全に起動する' 'Japanese fixed-path guidance'
check_text docs/database.md '## Direct Write Warning' 'English SQLite warning'
check_text docs/database.ja.md '## 直接書き込みの警告' 'Japanese SQLite warning'
check_text internal/ctxexec/ctxexec.go 'var ExecutablePath = "/usr/local/bin/ctx"' 'default ctx executable is fixed'

for package in xssh xftp xsmb xscp xgobuster xwebshell; do
  if find "internal/${package}" -type f -name '*.go' ! -name '*_test.go' \
    -exec grep -l '"req/internal/ctxapi"' {} + | grep . >/dev/null; then
    ok "${package} uses shared ctx JSON client"
  else
    ng "${package} does not use shared ctx JSON client"
  fi
done

legacy_calls="$(grep -R -n -E 'exec\.Command\("ctx"|\.Output\("ctx"|\.Run\("ctx"' cmd internal --include='*.go' 2>/dev/null || true)"
if [ -z "${legacy_calls}" ]; then
  ok 'no PATH-based ctx process calls'
else
  ng 'PATH-based ctx process calls remain'
  printf '%s\n' "${legacy_calls}"
fi

if output="$(./scripts/shared/check-package-config.sh 2>&1)"; then
  ok 'ctx Debian dependencies and package configuration'
else
  ng 'ctx Debian dependencies or package configuration'
  printf '%s\n' "${output}"
fi

if output="$(go test ./internal/ctx ./internal/ctxapi ./internal/ctxexec 2>&1)"; then
  ok 'ctx public contract and integration tests'
else
  ng 'ctx public contract or integration tests'
  printf '%s\n' "${output}"
fi

printf '\n'
if [ "${FAILED}" -ne 0 ]; then
  printf 'ctx integration audit failed.\n' >&2
  exit 1
fi

printf 'ctx integration audit passed.\n'
