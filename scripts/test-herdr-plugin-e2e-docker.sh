#!/bin/sh
set -eu

command -v docker >/dev/null 2>&1 || {
  printf 'error: docker is required for the isolated live Herdr tier\n' >&2
  exit 1
}
docker info >/dev/null 2>&1 || {
  printf 'error: the Docker daemon is not available\n' >&2
  exit 1
}

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tag=hwt-herdr-e2e:$$
cleanup() {
  status=$?
  docker image rm "$tag" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT INT TERM

docker build --tag "$tag" --file "$repo_root/e2e/herdr/Dockerfile" "$repo_root"
docker run --rm --network none "$tag"
