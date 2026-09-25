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

script_path=$0
case $script_path in
  /*) ;;
  *) script_path=$(CDPATH= cd -- "$(dirname -- "$script_path")" && pwd)/$(basename -- "$script_path") || exit 1 ;;
esac
link_count=0
while [ -L "$script_path" ]; do
  link_count=$((link_count + 1))
  [ "$link_count" -le 40 ] || {
    printf 'install: too many symlink redirects\n' >&2
    exit 1
  }
  link_target=$(readlink "$script_path") || exit 1
  case $link_target in
    /*) script_path=$link_target ;;
    *) script_path=$(CDPATH= cd -- "$(dirname -- "$script_path")" && pwd)/$link_target || exit 1 ;;
  esac
done
script_dir=$(CDPATH= cd -- "$(dirname -- "$script_path")" && pwd) || exit 1
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

codex_home=${CODEX_HOME:-}
claude_home=${CLAUDE_HOME:-}

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

resolve_home() {
  configured=$1
  suffix=$2
  host_home=$3
  if [ -n "$configured" ]; then
    printf '%s\n' "$configured"
  elif [ -n "${HOME:-}" ]; then
    printf '%s/%s\n' "$HOME" "$suffix"
  else
    printf 'install: HOME is required when %s is unset\n' "$host_home" >&2
    return 1
  fi
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
    printf 'install: failed to install %s; restored prior root from %s\n' "$target" "$backup" >&2
  else
    printf 'install: failed to install %s\n' "$target" >&2
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
  codex)
    codex_home=$(resolve_home "$codex_home" .agents CODEX_HOME) || exit 1
    install_one "$codex_home/skills/agent-team"
    ;;
  claude)
    claude_home=$(resolve_home "$claude_home" .claude CLAUDE_HOME) || exit 1
    install_one "$claude_home/skills/agent-team"
    ;;
  both)
    codex_home=$(resolve_home "$codex_home" .agents CODEX_HOME) || exit 1
    claude_home=$(resolve_home "$claude_home" .claude CLAUDE_HOME) || exit 1
    install_one "$codex_home/skills/agent-team" &&
      install_one "$claude_home/skills/agent-team"
    ;;
esac
