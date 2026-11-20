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

# list: one catalogue, one claim set, symptom and connect present
out="$("$here/plugin" list --json --instance=fake-1)"
echo "$out" | grep -q '"scenarios"' || { echo "FAIL: no scenarios"; fail=1; }
echo "$out" | grep -q '"claims":\["dir:' || { echo "FAIL: dir claim"; fail=1; }
echo "$out" | grep -q 'fake-1' || { echo "FAIL: claim not instance-specific"; fail=1; }
echo "$out" | grep -q '"symptom"' || { echo "FAIL: no symptom"; fail=1; }
echo "$out" | grep -q '"connect"' || { echo "FAIL: no connect info"; fail=1; }

# a port claim appears only when the port is set
write_args '{"port":18999}'
"$here/plugin" list --json --instance=fake-1 --args=@"$args" | grep -q '"port:18999"' || { echo "FAIL: port claim"; fail=1; }
write_args '{"port":0}'

# up prints the symptom and connection JSON on stdout
up_out="$("$here/plugin" up --instance=fake-1 --args=@"$args" 2>/dev/null)"
echo "$up_out" | grep -q '"symptom"' || { echo "FAIL: up did not report a symptom"; fail=1; }
echo "$up_out" | grep -q '"connect"' || { echo "FAIL: up did not report connect info"; fail=1; }

chk "$here/plugin" check --instance=fake-1 --args=@"$args"
chk "$here/plugin" down --instance=fake-1 --args=@"$args"

# check without an environment is a check error (exit 2), not "not fixed"
chk "$here/plugin" up --instance=fake-2 --args=@"$args"
chk_fail "$here/plugin" check --instance=fake-9 --args=@"$args"
chk "$here/plugin" down --instance=fake-2 --args=@"$args"

# broken -> check exits 1 and still reports per-fault detail
write_args '{"broken":true}'
chk "$here/plugin" up --instance=fake-3 --args=@"$args"
out="$("$here/plugin" check --instance=fake-3 --args=@"$args" 2>/dev/null)"
rc=$?
[ "$rc" -eq 1 ] || { echo "FAIL: broken check must exit 1, got $rc"; fail=1; }
echo "$out" | grep -q '"passed":false' || { echo "FAIL: broken check must report passed=false"; fail=1; }
chk "$here/plugin" down --instance=fake-3 --args=@"$args"

# fail-up
write_args '{"fail-up":true}'
chk_fail "$here/plugin" up --instance=fake-4 --args=@"$args"

# fail-down
write_args '{"fail-down":true}'
chk "$here/plugin" up --instance=fake-5 --args=@"$args"
chk_fail "$here/plugin" down --instance=fake-5 --args=@"$args"

# leak-dir: down succeeds but leaves the directory behind
write_args '{"leak-dir":true}'
chk "$here/plugin" up --instance=fake-6 --args=@"$args"
chk "$here/plugin" down --instance=fake-6 --args=@"$args"
[ -d "$tmp/fake-state/fake-6" ] || { echo "FAIL: leak-dir did not leak"; fail=1; }

# down is idempotent
write_args '{}'
chk "$here/plugin" down --instance=fake-1 --args=@"$args"

# unknown subcommand
chk_fail "$here/plugin" bogus

[ "$fail" -eq 0 ] && echo "OK"
exit "$fail"
