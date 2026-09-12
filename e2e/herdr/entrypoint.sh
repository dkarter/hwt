#!/bin/sh
set -eu

root=/tmp/hwt-herdr-e2e
export HOME=$root/home
export XDG_CONFIG_HOME=$root/config
export XDG_STATE_HOME=$root/state
export XDG_DATA_HOME=$root/data
export XDG_CACHE_HOME=$root/cache
export HERDR_CONFIG_PATH=$root/config/herdr/config.toml
export HERDR_SESSION=hwt-e2e
export HERDR_ENV=1
export HWT_E2E_ISOLATED=1
export HWT_E2E_LIVE=1
export HWT_E2E_HWT_PATH=/workspace/hwt
export GOMODCACHE=/go/pkg/mod
mkdir -p "$HOME" "$XDG_CONFIG_HOME/herdr" "$XDG_STATE_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME" "$root/bin"

cat >"$root/bin/herdr" <<'EOF'
#!/bin/sh
exec /usr/local/bin/herdr-real --session hwt-e2e "$@"
EOF
chmod 0755 "$root/bin/herdr"
export PATH=$root/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

/usr/local/bin/herdr-real --session hwt-e2e server >"$root/server.log" 2>&1 &
server_pid=$!
cleanup() {
  status=$?
  kill "$server_pid" >/dev/null 2>&1 || true
  wait "$server_pid" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT INT TERM

attempt=0
until herdr status server >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 100 ]; then
    cat "$root/server.log" >&2
    exit 1
  fi
  sleep 0.1
done

case $HERDR_CONFIG_PATH:$XDG_CONFIG_HOME:$XDG_STATE_HOME:$XDG_DATA_HOME:$XDG_CACHE_HOME in
  /tmp/hwt-herdr-e2e/*:/tmp/hwt-herdr-e2e/*:/tmp/hwt-herdr-e2e/*:/tmp/hwt-herdr-e2e/*:/tmp/hwt-herdr-e2e/*) ;;
  *) echo 'refusing to mutate a non-isolated Herdr session' >&2; exit 1 ;;
esac
herdr plugin link /workspace/plugins/herdr --enabled
herdr workspace create --cwd /workspace --label hwt-e2e-control --focus >/dev/null

go test ./e2e -run TestHDR004_HDR005_HDR006_HDR007_HDR008_LiveHerdrPlugin -count=1 -v
