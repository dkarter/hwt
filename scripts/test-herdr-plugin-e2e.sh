#!/bin/sh
set -eu

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

for command in git go herdr jq; do
  command -v "$command" >/dev/null 2>&1 || fail "$command is required"
done

[ "${HERDR_ENV:-}" = 1 ] || fail "run this test inside a Herdr-managed pane"

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
plugin_root=$repo_root/plugins/herdr
source_workspace=
created_workspace=
created_path=
linked_plugin=false
temp_root=
original_workspace=

cleanup() {
  status=$?
  set +e
  if [ -n "$created_workspace" ]; then
    "$hwt_bin" remove --workspace "$created_workspace" --force --json >/dev/null 2>&1
  elif [ -n "$created_path" ] && [ -e "$created_path" ]; then
    git -C "$fixture" worktree remove --force "$created_path" >/dev/null 2>&1
  fi
  if [ -n "$source_workspace" ]; then
    herdr workspace close "$source_workspace" >/dev/null 2>&1
  fi
  if [ -n "$original_workspace" ]; then
    focused_workspace=$(herdr workspace list 2>/dev/null | jq -r '.result.workspaces[] | select(.focused) | .workspace_id' 2>/dev/null)
    if [ "$focused_workspace" != "$original_workspace" ]; then
      herdr workspace focus "$original_workspace" >/dev/null 2>&1
    fi
  fi
  if [ "$linked_plugin" = true ]; then
    herdr plugin unlink hwt.worktrees >/dev/null 2>&1
  fi
  if [ -n "$temp_root" ]; then
    rm -rf "$temp_root"
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

temp_root=$(mktemp -d "${TMPDIR:-/tmp}/hwt-herdr-e2e.XXXXXX")
fixture=$temp_root/repository
worktree_root=$temp_root/worktrees
hwt_bin=$temp_root/hwt
original_workspace=$(herdr workspace list | jq -er '.result.workspaces[] | select(.focused) | .workspace_id')

mkdir "$fixture"
cat >"$fixture/README.md" <<'EOF'
# HWT Herdr plugin end-to-end fixture
EOF
cat >"$fixture/.env.test" <<'EOF'
HWT_E2E=created
EOF
cat >"$fixture/.herdr-worktree.yaml" <<EOF
worktree_dir: $worktree_root
worktree_naming: full
files:
  copy:
    - .env.test
post_create:
  - printf 'hook-ran\\n' > .hwt-e2e-hook
EOF

git -C "$fixture" init -b main >/dev/null
git -C "$fixture" config user.name "HWT E2E"
git -C "$fixture" config user.email "hwt-e2e@example.invalid"
git -C "$fixture" add -f README.md .env.test .herdr-worktree.yaml
git -C "$fixture" commit -m "test: initialize fixture" >/dev/null
go build -o "$hwt_bin" "$repo_root/cmd/hwt"

plugin_json=$(herdr plugin list --plugin hwt.worktrees --json)
if [ "$(printf '%s' "$plugin_json" | jq '.result.plugins | length')" -eq 0 ]; then
  herdr plugin link "$plugin_root" >/dev/null
  linked_plugin=true
else
  manifest_path=$(printf '%s' "$plugin_json" | jq -r '.result.plugins[0].manifest_path')
  [ "$manifest_path" = "$plugin_root/herdr-plugin.toml" ] || fail "hwt.worktrees is linked from $manifest_path"
fi

source_json=$(herdr workspace create --cwd "$fixture" --label hwt-plugin-e2e --no-focus)
source_workspace=$(printf '%s' "$source_json" | jq -er '.result.workspace.workspace_id')
create_context=$(jq -cn \
  --arg workspace_id "$source_workspace" \
  --arg cwd "$fixture" \
  '{workspace_id: $workspace_id, workspace_cwd: $cwd, focused_pane_cwd: $cwd}')

open_create_pane() {
  HERDR_PLUGIN_ID=hwt.worktrees \
    HERDR_PLUGIN_ROOT="$plugin_root" \
    HERDR_PLUGIN_CONTEXT_JSON="$create_context" \
    HWT_BIN_PATH="$hwt_bin" \
    HWT_HERDR_NO_FOCUS=1 \
    HWT_PLUGIN_PLACEMENT=tab \
    HWT_PLUGIN_FOCUS=false \
    HWT_PLUGIN_WORKSPACE="$source_workspace" \
    "$plugin_root/open-pane" create |
    jq -er '.result.plugin_pane.pane.pane_id'
}

assert_focus_unchanged() {
  focused_workspace=$(herdr workspace list | jq -er '.result.workspaces[] | select(.focused) | .workspace_id')
  [ "$focused_workspace" = "$original_workspace" ] || fail "focused workspace changed from $original_workspace to $focused_workspace"
}

assert_input_edit() {
  name=$1
  initial=$2
  expected=$3
  shift 3
  pane=$(open_create_pane)
  herdr pane wait-output "$pane" --match "Branch name" --timeout 10000 >/dev/null
  herdr pane send-text "$pane" "$initial"
  herdr pane send-keys "$pane" esc
  for key in "$@"; do
    herdr pane send-text "$pane" "$key"
  done
  if ! herdr pane wait-output "$pane" --regex "^> ${expected}$" --source visible --timeout 10000 --raw >/dev/null; then
    herdr pane read "$pane" --source visible --lines 20 >&2 || true
    fail "Vim input case $name did not produce $expected"
  fi
  herdr pane send-keys "$pane" esc
  herdr pane send-text "$pane" q
  attempt=0
  while [ "$attempt" -lt 50 ] && herdr pane get "$pane" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    sleep 0.05
  done
  if herdr pane get "$pane" >/dev/null 2>&1; then
    herdr plugin pane close "$pane" >/dev/null 2>&1 || true
    fail "Vim input case $name did not close"
  fi
  assert_focus_unchanged
}

printf 'Testing Vim motions, insertion commands, substitute, and delete motions...\n'
assert_input_edit e "one two" "onX two" 0 e s X
assert_input_edit b "one two" "one Xwo" b s X
assert_input_edit w "one two" "one Xwo" 0 w s X
assert_input_edit B "one two-three" "one Xwo-three" B s X
assert_input_edit E "one two-three" "one two-threX" 0 w E s X
assert_input_edit A "one" "oneX" A X
assert_input_edit I "  one" "  Xone" I X
assert_input_edit s "one" "onX" s X
assert_input_edit dw "one two" "two" 0 d w
assert_input_edit de "one two" " two" 0 d e
assert_input_edit db "one two-three" "one two-ree" b l l d b
assert_input_edit dB "one two-three" "one ree" b l l d B
assert_input_edit dE "alpha beta-gamma Z" "alpha  Z" 0 w d E

printf 'Creating a configured worktree through the plugin...\n'
create_pane=$(open_create_pane)
herdr pane wait-output "$create_pane" --match "Branch name" --timeout 10000 >/dev/null
herdr pane send-keys "$create_pane" esc
herdr pane wait-output "$create_pane" --match "NORMAL" --timeout 10000 >/dev/null
herdr pane send-text "$create_pane" A
herdr pane send-text "$create_pane" "feature/plugin e2e"
herdr pane wait-output "$create_pane" --regex '^> feature/plugin e2e$' --source visible --timeout 10000 --raw >/dev/null
herdr pane send-keys "$create_pane" enter
herdr pane wait-output "$create_pane" --match "Fuzzy search" --timeout 10000 >/dev/null
herdr pane send-text "$create_pane" ma
herdr pane wait-output "$create_pane" --regex '^> ma$' --source visible --timeout 10000 --raw >/dev/null
herdr pane send-keys "$create_pane" enter

attempt=0
while [ "$attempt" -lt 100 ]; do
  worktrees=$(herdr worktree list --cwd "$fixture")
  created_workspace=$(printf '%s' "$worktrees" | jq -r '.result.worktrees[] | select(.branch == "feature/plugin-e2e") | .open_workspace_id // empty')
  created_path=$(printf '%s' "$worktrees" | jq -r '.result.worktrees[] | select(.branch == "feature/plugin-e2e") | .path // empty')
  if [ -n "$created_workspace" ] && [ -n "$created_path" ] &&
    [ -f "$created_path/.env.test" ] && [ -f "$created_path/.hwt-e2e-hook" ] &&
    [ "$(git -C "$created_path" config --local --get branch.feature/plugin-e2e.herdr-base 2>/dev/null || true)" = main ]; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 0.1
done
[ -n "$created_workspace" ] || fail "created workspace was not reported"
[ -f "$created_path/.env.test" ] || fail "configured file was not copied"
[ "$(cat "$created_path/.env.test")" = "HWT_E2E=created" ] || fail "copied file content is incorrect"
[ "$(cat "$created_path/.hwt-e2e-hook")" = "hook-ran" ] || fail "post-create hook did not run"
[ "$(git -C "$created_path" config --local --get branch.feature/plugin-e2e.herdr-base)" = main ] || fail "base branch was not recorded"
assert_focus_unchanged

remove_context=$(jq -cn \
  --arg workspace_id "$created_workspace" \
  --arg cwd "$created_path" \
  '{workspace_id: $workspace_id, workspace_cwd: $cwd, focused_pane_cwd: $cwd}')

printf 'Removing the dirty worktree through the plugin force-confirmation flow...\n'
remove_pane_json=$(
  HERDR_PLUGIN_ID=hwt.worktrees \
    HERDR_PLUGIN_ROOT="$plugin_root" \
    HERDR_PLUGIN_CONTEXT_JSON="$remove_context" \
    HWT_BIN_PATH="$hwt_bin" \
    HWT_PLUGIN_PLACEMENT=tab \
    HWT_PLUGIN_FOCUS=false \
    HWT_PLUGIN_WORKSPACE="$source_workspace" \
    "$plugin_root/open-pane" remove
)
remove_pane=$(printf '%s' "$remove_pane_json" | jq -er '.result.plugin_pane.pane.pane_id')
herdr pane wait-output "$remove_pane" --match "Remove worktree" --timeout 10000 >/dev/null
herdr pane send-text "$remove_pane" h
herdr pane send-keys "$remove_pane" enter
herdr pane wait-output "$remove_pane" --match "Force removal and discard uncommitted files?" --timeout 10000 >/dev/null
herdr pane send-text "$remove_pane" h
herdr pane send-keys "$remove_pane" enter

attempt=0
while [ "$attempt" -lt 100 ]; do
  workspace_open=false
  metadata_registered=false
  if herdr workspace get "$created_workspace" >/dev/null 2>&1; then
    workspace_open=true
  fi
  if git -C "$fixture" worktree list --porcelain | grep -F "$created_path" >/dev/null 2>&1; then
    metadata_registered=true
  fi
  if [ ! -e "$created_path" ] && [ "$workspace_open" = false ] && [ "$metadata_registered" = false ]; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 0.1
done
[ ! -e "$created_path" ] || fail "worktree checkout was not deleted"
if herdr workspace get "$created_workspace" >/dev/null 2>&1; then
  fail "worktree workspace is still open"
fi
if git -C "$fixture" worktree list --porcelain | grep -F "$created_path" >/dev/null 2>&1; then
  git -C "$fixture" worktree list --porcelain >&2
  printf 'expected removed path: %s\n' "$created_path" >&2
  fail "Git worktree metadata is still registered"
fi
git -C "$fixture" show-ref --verify --quiet refs/heads/feature/plugin-e2e || fail "worktree branch was not preserved"
assert_focus_unchanged
created_workspace=
created_path=

printf 'Herdr plugin end-to-end test passed.\n'
