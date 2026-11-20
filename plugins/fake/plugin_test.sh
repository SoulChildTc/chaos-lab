#!/usr/bin/env bash
# Protocol self-test for the fake plugin. Runs the plugin directly, without the
# framework, so a broken protocol implementation is caught here.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export CHAOSLAB_HOME="$tmp"
args="$tmp/args.json"
fail=0

chk() {
  if ! "$@" >/dev/null 2>&1; then
    echo "FAIL: $*"
    fail=1
  fi
}
chk_fail() {
  if "$@" >/dev/null 2>&1; then
    echo "FAIL (expected non-zero): $*"
    fail=1
  fi
}
write_args() { printf '%s' "$1" > "$args"; }

write_args '{"port":0,"fail-up":false,"fail-down":false,"leak-dir":false,"broken":false}'

# schema
"$here/plugin" schema --json | grep -q '"name":"fake"' || { echo "FAIL: schema name"; fail=1; }
"$here/plugin" schema --json | grep -q '"port"' || { echo "FAIL: schema arg port"; fail=1; }

# list renders claims for the given instance
out="$("$here/plugin" list --json --instance=fake-env-1)"
echo "$out" | grep -q '"claims":\["dir:' || { echo "FAIL: dir claim"; fail=1; }
echo "$out" | grep -q 'fake-env-1' || { echo "FAIL: claim not instance-specific"; fail=1; }

# list with a port declares a port claim
write_args '{"port":18999}'
"$here/plugin" list --json --instance=fake-env-1 --args=@"$args" | grep -q '"port:18999"' || { echo "FAIL: port claim"; fail=1; }
write_args '{"port":0}'

# happy path
chk "$here/plugin" up env --instance=fake-env-1 --args=@"$args"
chk "$here/plugin" check env --instance=fake-env-1 --args=@"$args"
chk "$here/plugin" down env --instance=fake-env-1 --args=@"$args"

# up is idempotent enough to be retried after a failure
chk "$here/plugin" up env --instance=fake-env-2 --args=@"$args"
chk_fail "$here/plugin" check env --instance=fake-env-9 --args=@"$args"   # no env -> exit 2

# broken -> check exits 1, not 0 and not a crash
write_args '{"broken":true}'
chk "$here/plugin" up env --instance=fake-env-3 --args=@"$args"
"$here/plugin" check env --instance=fake-env-3 --args=@"$args" >/dev/null 2>&1
[ $? -eq 1 ] || { echo "FAIL: broken check must exit 1"; fail=1; }

# fail-up
write_args '{"fail-up":true}'
chk_fail "$here/plugin" up env --instance=fake-env-4 --args=@"$args"

# fail-down
write_args '{"fail-down":true}'
chk "$here/plugin" up env --instance=fake-env-5 --args=@"$args"
chk_fail "$here/plugin" down env --instance=fake-env-5 --args=@"$args"

# leak-dir: down succeeds but leaves the directory behind
write_args '{"leak-dir":true}'
chk "$here/plugin" up env --instance=fake-env-6 --args=@"$args"
chk "$here/plugin" down env --instance=fake-env-6 --args=@"$args"
[ -d "$tmp/fake-state/fake-env-6" ] || { echo "FAIL: leak-dir did not leak"; fail=1; }

# down is idempotent: running it twice is a success
write_args '{}'
chk "$here/plugin" down env --instance=fake-env-1 --args=@"$args"

# unknown subcommand
chk_fail "$here/plugin" bogus

if [ "$fail" -eq 0 ]; then
  echo "OK"
fi
exit "$fail"
