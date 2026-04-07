#!/usr/bin/env bash

safe_agentic_repo_path_from_url() {
  local repo_url="$1"
  local clone_path owner repo

  case "$repo_url" in -*) return 1 ;; esac

  clone_path="${repo_url%.git}"

  case "$clone_path" in
    https://*|ssh://*) clone_path="${clone_path#*://*/}" ;;
    *:*/*) clone_path="${clone_path##*:}" ;;
    *) return 1 ;;
  esac

  [[ "$clone_path" == */* ]] || return 1
  owner="${clone_path%/*}"
  repo="${clone_path#*/}"
  [[ "$owner" == */* ]] && return 1

  case "$owner" in ""|.*|-*) return 1 ;; esac
  case "$repo" in ""|.*|-*) return 1 ;; esac
  [[ "$owner" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1
  [[ "$repo" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1

  printf '%s/%s\n' "$owner" "$repo"
}

repo_path_from_url() {
  safe_agentic_repo_path_from_url "$@"
}

repo_clone_path() {
  safe_agentic_repo_path_from_url "$@"
}
