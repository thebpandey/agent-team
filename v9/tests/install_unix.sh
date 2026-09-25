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

# A new explicit Codex home must receive the packaged skill and report its root.
codex_home="$test_root/new-codex"
output=$(bash "$installer" codex --codex-home "$codex_home")
codex_target="$codex_home/skills/agent-team"
assert_file "$codex_target/SKILL.md"
assert_same "$source_root/SKILL.md" "$codex_target/SKILL.md"
assert_output_line "$output" "installed: $codex_target"

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

printf 'PASS: Unix installer\n'
