#!/usr/bin/env bash

set -euo pipefail

root=$(git rev-parse --show-toplevel)
website="$root/website"
docs="$website/src/content/docs"
temp=$(mktemp -d "${TMPDIR:-/tmp}/hwt-versioned-site.XXXXXX")
dev_docs="$temp/dev-docs"
stable_tag=""
stable_routes="/|"

while IFS= read -r tag; do
  if [[ "$tag" != *-* ]]; then
    stable_tag="$tag"
    break
  fi
done < <(git tag --list 'v[0-9]*' --sort=-v:refname)

if [[ -z "$stable_tag" ]]; then
  echo "no stable documentation tag found" >&2
  exit 1
fi
stable_version=${stable_tag#v}

while IFS= read -r file; do
  route=${file#website/src/content/docs/}
  route=${route%.*}
  route=${route%/index}
  if [[ "$route" == "index" ]]; then
    route=""
  fi
  stable_routes+="/docs/${route:+$route/}|"
done < <(git ls-tree -r --name-only "$stable_tag" website/src/content/docs)

restore_docs() {
  if [[ ! -d "$dev_docs" ]]; then
    return
  fi

  if [[ -d "$docs" ]]; then
    mv "$docs" "$temp/interrupted-docs"
  fi
  mv "$dev_docs" "$docs"
}

finish() {
  restore_docs
  if command -v trash >/dev/null; then
    trash "$temp"
  fi
}
trap finish EXIT

mv "$docs" "$dev_docs"
mkdir -p "$temp/stable-source"
git archive "$stable_tag" website/src/content/docs | tar -x -C "$temp/stable-source"
mv "$temp/stable-source/website/src/content/docs" "$docs"

HWT_DOCS_VERSION="$stable_version" HWT_STABLE_VERSION="$stable_version" \
  HWT_STABLE_ROUTES="$stable_routes" HWT_SITE_BASE=/ \
  aube --dir "$website" run build
mv "$website/dist" "$temp/stable-dist"

mv "$docs" "$temp/stable-docs"
mv "$dev_docs" "$docs"

HWT_DOCS_VERSION=dev HWT_STABLE_VERSION="$stable_version" \
  HWT_STABLE_ROUTES="$stable_routes" HWT_SITE_BASE=/dev \
  aube --dir "$website" run build
mv "$website/dist" "$temp/dev-dist"

mv "$temp/stable-dist" "$website/dist"
mkdir -p "$website/dist/dev"
cp -R "$temp/dev-dist/." "$website/dist/dev/"
