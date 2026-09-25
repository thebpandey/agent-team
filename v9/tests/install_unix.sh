#!/usr/bin/env bash
set -euo pipefail

# Each assertion catches a consumer-visible installer break: missing host copy,
# lost user file on update, or a copy verification failure that skips rollback.
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
installer="$repo/v9/install.sh"
source_root="$repo/v9/agent-team"
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_file() {
  test -f "$1" || fail "expected file: $1"
}

assert_same() {
  cmp -s "$1" "$2" || fail "files differ: $1 $2"
}

assert_output_line() {
  printf '%s\n' "$1" | grep -Fqx "$2" || fail "missing output line: $2"
}

assert_output_contains() {
  grep -Fq "$2" "$1" || fail "missing output text: $2"
}

# A new explicit Codex home must receive the packaged skill and report its root.
codex_home="$test_root/new-codex"
output=$(bash "$installer" codex --codex-home "$codex_home")
codex_target="$codex_home/skills/agent-team"
assert_file "$codex_target/SKILL.md"
assert_same "$source_root/SKILL.md" "$codex_target/SKILL.md"
assert_output_line "$output" "installed: $codex_target"

# Explicit targets must not require HOME or either host's environment default.
unset_home_target="$test_root/unset-home-codex/skills/agent-team"
output=$(env -u HOME -u CODEX_HOME -u CLAUDE_HOME bash "$installer" codex --codex-home "$test_root/unset-home-codex")
assert_file "$unset_home_target/SKILL.md"
assert_output_line "$output" "installed: $unset_home_target"

# Calling the installer through a symlink must still find the packaged payload.
link_dir="$test_root/link-bin"
mkdir "$link_dir"
ln -s "$installer" "$link_dir/agent-team-install"
link_target="$test_root/link-codex/skills/agent-team"
output=$(bash "$link_dir/agent-team-install" codex --codex-home "$test_root/link-codex")
assert_file "$link_target/SKILL.md"
assert_output_line "$output" "installed: $link_target"

# both must install separately into both supplied disposable homes.
both_codex="$test_root/both-codex"
both_claude="$test_root/both-claude"
output=$(bash "$installer" both --codex-home "$both_codex" --claude-home "$both_claude")
assert_file "$both_codex/skills/agent-team/SKILL.md"
assert_file "$both_claude/skills/agent-team/SKILL.md"
assert_output_line "$output" "installed: $both_codex/skills/agent-team"
assert_output_line "$output" "installed: $both_claude/skills/agent-team"

# Updating replaces the packaged root, preserves unknown bytes only in the
# reported whole-root backup, and never merges them into the new installation.
printf 'user bytes \001\377\n' > "$codex_target/user-only.bin"
output=$(bash "$installer" codex --codex-home "$codex_home")
backup=$(printf '%s\n' "$output" | sed -n 's/^backup: //p')
test -n "$backup" || fail 'update did not report a backup'
assert_file "$backup/user-only.bin"
assert_same "$backup/user-only.bin" <(printf 'user bytes \001\377\n')
test ! -e "$codex_target/user-only.bin" || fail 'unknown file leaked into new root'

# Removing a copied package file is a failed copy: the old whole root must be
# restored, including its unknown bytes, rather than leaving a partial install.
rollback_home="$test_root/rollback-codex"
rollback_target="$rollback_home/skills/agent-team"
mkdir -p "$rollback_target"
printf 'keep these bytes\n' > "$rollback_target/user-only.bin"
fake_bin="$test_root/fake-bin"
mkdir "$fake_bin"
cat > "$fake_bin/cp" <<'EOF'
#!/bin/sh
/bin/cp "$@" || exit $?
for arg do last=$arg; done
rm -f "$last/SKILL.md"
EOF
chmod +x "$fake_bin/cp"
if PATH="$fake_bin:$PATH" bash "$installer" codex --codex-home "$rollback_home" >"$test_root/failure.out" 2>&1; then
  fail 'copy missing SKILL.md unexpectedly succeeded'
fi
assert_file "$rollback_target/user-only.bin"
assert_same "$rollback_target/user-only.bin" <(printf 'keep these bytes\n')
test ! -e "$rollback_target/references" || fail 'partial new root remained after rollback'

# In both mode, a Claude-only copy failure leaves the completed Codex target
# alone, restores Claude's prior root, and identifies that restored target.
both_rollback_codex="$test_root/both-rollback-codex"
both_rollback_claude="$test_root/both-rollback-claude"
both_rollback_target="$both_rollback_claude/skills/agent-team"
mkdir -p "$both_rollback_target"
printf 'Claude bytes to restore\n' > "$both_rollback_target/user-only.bin"
cat > "$fake_bin/cp" <<'EOF'
#!/bin/sh
/bin/cp "$@" || exit $?
for arg do last=$arg; done
case $last in
  "$FAIL_COPY_TARGET".install.*) rm -f "$last/SKILL.md" ;;
esac
EOF
chmod +x "$fake_bin/cp"
both_failure="$test_root/both-failure.out"
if FAIL_COPY_TARGET="$both_rollback_target" PATH="$fake_bin:$PATH" bash "$installer" both --codex-home "$both_rollback_codex" --claude-home "$both_rollback_claude" >"$both_failure" 2>&1; then
  fail 'Claude-only failed copy unexpectedly succeeded'
fi
assert_file "$both_rollback_codex/skills/agent-team/SKILL.md"
assert_file "$both_rollback_target/user-only.bin"
assert_same "$both_rollback_target/user-only.bin" <(printf 'Claude bytes to restore\n')
assert_output_contains "$both_failure" "install: failed to install $both_rollback_target; restored prior root from "

printf 'PASS: Unix installer\n'
