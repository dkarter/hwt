#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 5 ]]; then
  echo "usage: $0 STABLE_VERSION YYYYMMDD RUN_NUMBER RUN_ATTEMPT COMMIT_SHA" >&2
  exit 2
fi

stable_version=$1
build_date=$2
run_number=$3
run_attempt=$4
commit_sha=$5

if [[ ! $stable_version =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "stable version must be a semantic version without a leading v: $stable_version" >&2
  exit 1
fi

parsed_date=
if [[ $build_date =~ ^[0-9]{8}$ ]]; then
  parsed_date=$(date -u -j -f '%Y%m%d' "$build_date" '+%Y%m%d' 2>/dev/null) ||
    parsed_date=$(date -u -d "$build_date" '+%Y%m%d' 2>/dev/null) || true
fi
if [[ $parsed_date != "$build_date" ]]; then
  echo "build date must be a valid YYYYMMDD date: $build_date" >&2
  exit 1
fi

if [[ ! $run_number =~ ^[1-9][0-9]*$ ]] || [[ ! $run_attempt =~ ^[1-9][0-9]*$ ]]; then
  echo "run number and attempt must be positive integers" >&2
  exit 1
fi

if [[ ! $commit_sha =~ ^[0-9a-fA-F]{7,40}$ ]]; then
  echo "commit SHA must contain 7 to 40 hexadecimal characters: $commit_sha" >&2
  exit 1
fi

IFS=. read -r major minor patch <<<"$stable_version"
short_sha=${commit_sha:0:7}
short_sha=$(printf '%s' "$short_sha" | tr '[:upper:]' '[:lower:]')

printf 'v%s.%s.%s-dev.%s.%s.%s.g%s\n' \
  "$major" "$minor" "$((10#$patch + 1))" "$build_date" "$run_number" "$run_attempt" "$short_sha"
