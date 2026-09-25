#!/bin/sh
set -u

usage() {
  printf '%s\n' "usage: $0 codex|claude|both [--codex-home PATH] [--claude-home PATH]" >&2
  exit 2
}

die() {
  printf 'install: %s\n' "$*" >&2
  return 1
}

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || exit 1
source_root=$script_dir/agent-team
[ -d "$source_root" ] || {
  printf 'install: packaged agent-team directory is missing\n' >&2
  exit 1
}

[ "$#" -gt 0 ] || usage
host=$1
shift
case $host in
  codex|claude|both) ;;
  *) usage ;;
esac

if [ -n "${CODEX_HOME:-}" ]; then
  codex_home=$CODEX_HOME
else
  : "${HOME:?install: HOME is required when CODEX_HOME is unset}"
  codex_home=$HOME/.agents
fi
if [ -n "${CLAUDE_HOME:-}" ]; then
  claude_home=$CLAUDE_HOME
else
  : "${HOME:?install: HOME is required when CLAUDE_HOME is unset}"
  claude_home=$HOME/.claude
fi

while [ "$#" -gt 0 ]; do
  case $1 in
    --codex-home)
      [ "$#" -ge 2 ] || usage
      codex_home=$2
      shift 2
      ;;
    --claude-home)
      [ "$#" -ge 2 ] || usage
      claude_home=$2
      shift 2
      ;;
    *) usage ;;
  esac
done

verify_copy() (
  cd "$1" || exit 1
  destination=$2
  find . -type f -exec sh -c '
    destination=$1
    shift
    for file do
      [ -f "$destination/$file" ] || exit 1
      source_sum=$(cksum "$file") || exit 1
      copied_sum=$(cksum "$destination/$file") || exit 1
      source_sum=${source_sum%% *}
      copied_sum=${copied_sum%% *}
      [ "$source_sum" = "$copied_sum" ] || exit 1
    done
  ' sh "$destination" {} +
)

next_sibling() {
  parent=$1
  name=$2
  label=$3
  candidate=$parent/$name.$label.$$
  suffix=0
  while [ -e "$candidate" ] || [ -L "$candidate" ]; do
    suffix=$((suffix + 1))
    candidate=$parent/$name.$label.$$.${suffix}
  done
  printf '%s\n' "$candidate"
}

restore() {
  staged=$1
  backup=$2
  target=$3
  had_backup=$4
  rm -rf "$staged"
  if [ "$had_backup" = yes ]; then
    mv "$backup" "$target" || {
      printf 'install: failed to restore %s from %s\n' "$target" "$backup" >&2
      return 1
    }
  fi
}

install_one() {
  target=$1
  [ ! -L "$target" ] || die "refusing symlink target root: $target" || return 1

  case $target in
    */*) parent=${target%/*}; name=${target##*/}; [ -n "$parent" ] || parent=/ ;;
    *) parent=.; name=$target ;;
  esac
  mkdir -p "$parent" || return 1

  backup=
  had_backup=no
  if [ -e "$target" ]; then
    backup=$(next_sibling "$parent" "$name" backup) || return 1
    mv "$target" "$backup" || return 1
    had_backup=yes
  fi

  staged=$(next_sibling "$parent" "$name" install) || {
    restore '' "$backup" "$target" "$had_backup"
    return 1
  }
  if ! mkdir "$staged" || ! cp -R "$source_root/." "$staged" || ! verify_copy "$source_root" "$staged" || ! mv "$staged" "$target"; then
    restore "$staged" "$backup" "$target" "$had_backup"
    return 1
  fi

  printf 'installed: %s\n' "$target"
  if [ "$had_backup" = yes ]; then
    printf 'backup: %s\n' "$backup"
  fi
}

case $host in
  codex) install_one "$codex_home/skills/agent-team" ;;
  claude) install_one "$claude_home/skills/agent-team" ;;
  both)
    install_one "$codex_home/skills/agent-team" &&
      install_one "$claude_home/skills/agent-team"
    ;;
esac
