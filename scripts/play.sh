#!/usr/bin/env bash
# One command for a phone test.
#
#   scripts/play.sh you@example.com      # starts everything and shows the QR code
#   scripts/play.sh say "hello"          # from another terminal: send a message as you
#   scripts/play.sh stop                 # if anything is left running
#
# It starts Postgres and Redis, builds and starts the hub, signs you in by
# reading the code from the hub's own log, creates (or reuses) an Echo agent
# and runs it, then starts the app's dev server with the hub's address on
# your wifi filled in. Sign-in codes for the phone are printed here as they
# happen. Ctrl+C stops all of it.
#
# Where the app runs is TARGET: phone (default; a QR code for Expo Go),
# web (a tab in this laptop's browser), or emulator (the Android emulator,
# installed with make emulator-install).
#
# Settings: CUCKOO_HUB_PORT (default 8080), CUCKOO_HUB_HOST (your wifi
# address; detected), PLAY_NO_APP=1 to start only the servers.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE="$ROOT/.play"
HUB_LOG="$STATE/hub.log"
ECHO_LOG="$STATE/echo.log"
ENV_FILE="$STATE/env"
PORT="${CUCKOO_HUB_PORT:-8080}"
HUB="http://localhost:$PORT"
J='Content-Type: application/json'
mkdir -p "$STATE"

json() { python3 -c "import sys, json; d = json.load(sys.stdin); $1"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
say_hdr() { printf '\n\033[1m%s\033[0m\n' "$*"; }

# fail_on_error prints the hub's error and exits if a response is an error envelope.
fail_on_error() {
  local body="$1" what="$2"
  if printf '%s' "$body" | json 'sys.exit(0 if "error" in d else 1)' 2>/dev/null; then
    printf '%s' "$body" | json 'print("%s: %s" % (d["error"]["code"], d["error"]["message"]))' >&2
    die "$what failed"
  fi
}

wait_for() {
  local url="$1" secs="$2"
  for _ in $(seq 1 $((secs * 4))); do
    curl -sf "$url" >/dev/null 2>&1 && return 0
    sleep 0.25
  done
  return 1
}

# ---------------------------------------------------------------- stop / say

stop() {
  local pid
  for port in "$PORT" 8081; do
    pid=$(ss -ltnp 2>/dev/null | grep ":$port " | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1 || true)
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null && echo "stopped what was listening on :$port"
    fi
  done
  pkill -f "venv/bin/python echo.py" 2>/dev/null && echo "stopped the echo agent" || true
  pkill -f "tail -n0 -F $HUB_LOG" 2>/dev/null || true
  echo "done"
}

say() {
  [ -f "$ENV_FILE" ] || die "nothing is set up yet; run: make play EMAIL=you@example.com"
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  local body
  body=$(curl -s -H "X-Dev-User: $PLAY_USER_ID" -X POST "$HUB/v1/client/conversations/$PLAY_CONV_ID/messages" \
    -H "$J" -d "$(printf '%s' "$1" | python3 -c 'import sys, json; print(json.dumps({"text": sys.stdin.read()}))')")
  fail_on_error "$body" "send"
  printf '%s' "$body" | json 'm = d["message"]; print("sent %s (%s): %r" % (m["id"], m["delivery_status"], m["body"]["text"]))'
}

# ---------------------------------------------------------------- start

pids=()
cleanup() {
  local code=$?
  trap - EXIT
  echo
  echo "Stopping the agent and the hub ..."
  pkill -f "tail -n0 -F $HUB_LOG" 2>/dev/null || true
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  exit "$code"
}

external_hub=0

start_hub() {
  if curl -sf "$HUB/healthz" >/dev/null 2>&1; then
    external_hub=1
    echo "A hub is already running on :$PORT; using it. Its sign-in codes appear in its own terminal."
    return
  fi
  say_hdr "Starting Postgres and Redis"
  (cd "$ROOT" && make -s up)
  say_hdr "Building and starting the hub on :$PORT"
  (cd "$ROOT/server" && go build -o "$ROOT/bin/cuckoo" ./cmd/cuckoo)
  : > "$HUB_LOG"
  (cd "$ROOT/server" && CUCKOO_ENV=dev CUCKOO_MAIL=console CUCKOO_HTTP_ADDR=":$PORT" exec "$ROOT/bin/cuckoo" >> "$HUB_LOG" 2>&1) &
  pids+=("$!")
  wait_for "$HUB/healthz" 30 || { tail -5 "$HUB_LOG" >&2; die "the hub did not start; log: $HUB_LOG"; }
  echo "hub is up (log: $HUB_LOG)"
}

# code_for reads the newest sign-in code the hub mailed to an address.
code_for() {
  grep -F "\"to\":\"$1\"" "$HUB_LOG" | grep -o 'sign-in code is [0-9]*' | tail -1 | grep -o '[0-9]*' || true
}

sign_in() {
  local email="$1" before code body
  say_hdr "Signing in as $email"
  before=$(grep -c 'sign-in code is' "$HUB_LOG" 2>/dev/null || true)
  body=$(curl -s -X POST "$HUB/v1/auth/email/start" -H "$J" -d "{\"email\":\"$email\"}")
  fail_on_error "$body" "requesting a code"
  if [ "$external_hub" = 1 ]; then
    echo "Look at the hub's terminal for a line containing: Your sign-in code is ......"
    read -r -p "Type that six-digit code here: " code
  else
    code=""
    for _ in $(seq 1 40); do
      if [ "$(grep -c 'sign-in code is' "$HUB_LOG")" -gt "$before" ]; then
        code=$(code_for "$email")
        [ -n "$code" ] && break
      fi
      sleep 0.25
    done
    [ -n "$code" ] || die "no sign-in code appeared in $HUB_LOG"
    echo "read the code from the hub's log"
  fi
  body=$(curl -s -X POST "$HUB/v1/auth/email/verify" -H "$J" \
    -d "{\"email\":\"$email\",\"code\":\"$code\",\"device_name\":\"play script\"}")
  fail_on_error "$body" "sign-in"
  USER_ID=$(printf '%s' "$body" | json 'print(d["user"]["id"])')
  echo "signed in as $USER_ID"
}

set_up_agent() {
  local h="X-Dev-User: $USER_ID" body handle
  say_hdr "Your Echo agent"
  AGENT_ID=$(curl -s -H "$h" "$HUB/v1/mgmt/agents" | json 'a = [x for x in d["agents"] if x["display_name"] == "Echo"]; print(a[0]["id"] if a else "")')
  if [ -z "$AGENT_ID" ]; then
    handle="echo-$(head -c 4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    body=$(curl -s -H "$h" -X POST "$HUB/v1/mgmt/agents" -H "$J" \
      -d "{\"handle\":\"$handle\",\"display_name\":\"Echo\",\"description\":\"Says what you said.\"}")
    fail_on_error "$body" "creating the agent"
    AGENT_ID=$(printf '%s' "$body" | json 'print(d["agent"]["id"])')
    echo "created $AGENT_ID"
  else
    echo "reusing $AGENT_ID"
  fi
  body=$(curl -s -H "$h" -X POST "$HUB/v1/mgmt/agents/$AGENT_ID/binding" -H "$J" -d '{"mode":"socket"}')
  fail_on_error "$body" "creating the binding"
  SECRET=$(printf '%s' "$body" | json 'print(d["secret"])')
  CONV_ID=$(curl -s -H "$h" "$HUB/v1/client/conversations" | json "
c = [x for x in d['conversations'] if any(p['kind'] == 'agent' and p['id'] == '$AGENT_ID' for p in x['participants'])]
print(c[0]['id'] if c else '')")
  [ -n "$CONV_ID" ] || die "could not find the chat with the agent"
  printf 'PLAY_USER_ID=%s\nPLAY_AGENT_ID=%s\nPLAY_CONV_ID=%s\n' "$USER_ID" "$AGENT_ID" "$CONV_ID" > "$ENV_FILE"
}

run_agent() {
  local dir="$ROOT/examples/echo"
  if [ ! -x "$dir/.venv/bin/python" ]; then
    echo "setting up the agent's Python environment (once) ..."
    python3 -m venv "$dir/.venv"
    "$dir/.venv/bin/pip" install -q -e "$ROOT/sdk/python"
  fi
  : > "$ECHO_LOG"
  (cd "$dir" && CUCKOO_SECRET="$SECRET" CUCKOO_HUB="$HUB" exec .venv/bin/python echo.py >> "$ECHO_LOG" 2>&1) &
  pids+=("$!")
  for _ in $(seq 1 40); do
    grep -q 'connected to' "$ECHO_LOG" 2>/dev/null && break
    sleep 0.25
  done
  grep -q 'connected to' "$ECHO_LOG" || { tail -3 "$ECHO_LOG" >&2; die "the echo agent did not connect; log: $ECHO_LOG"; }
  echo "echo agent is running (log: $ECHO_LOG)"
}

# watch_codes prints every sign-in code the hub issues from now on, so the
# phone's code shows up right here.
watch_codes() {
  [ "$external_hub" = 1 ] && return
  (tail -n0 -F "$HUB_LOG" 2>/dev/null | while IFS= read -r line; do
    case "$line" in
      *'sign-in code is'*)
        to=$(printf '%s' "$line" | grep -o '"to":"[^"]*"' | cut -d'"' -f4)
        code=$(printf '%s' "$line" | grep -o 'sign-in code is [0-9]*' | grep -o '[0-9]*')
        printf '\n\033[1;32m>>> Sign-in code for %s: %s <<<\033[0m\n\n' "$to" "$code"
        ;;
    esac
  done) &
  pids+=("$!")
}

# lan_address is the laptop's address on the network the phone is on: the
# one the laptop itself uses to reach the internet, failing that the first
# real interface, never a Docker bridge.
lan_address() {
  local a
  a=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") print $(i + 1)}' | head -1 || true)
  [ -n "$a" ] && { echo "$a"; return; }
  a=$(ip -4 -o addr show scope global 2>/dev/null | grep -v -E ' (docker|br-|veth|virbr)' | awk '{print $4}' | cut -d/ -f1 | head -1 || true)
  [ -n "$a" ] && { echo "$a"; return; }
  hostname -I 2>/dev/null | awk '{print $1}' || true
}

start_app() {
  local target="${TARGET:-phone}" host
  case "$target" in
    web) host="localhost" ;;
    # 10.0.2.2 is how an Android emulator names the machine it runs on.
    emulator) host="10.0.2.2" ;;
    phone) host="${CUCKOO_HUB_HOST:-$(lan_address)}" ;;
    *) die "TARGET must be phone, web or emulator (got $target)" ;;
  esac
  [ -n "$host" ] || die "could not detect your wifi address; is the laptop connected? Or set CUCKOO_HUB_HOST=<address>"
  if [ ! -d "$ROOT/apps/mobile/node_modules" ]; then
    say_hdr "Installing the app's packages (once)"
    (cd "$ROOT/apps/mobile" && npm install --no-audit --no-fund)
  fi
  # A host firewall silently drops the phone's requests; Expo Go then loads
  # forever. Reading the rules needs root, so just say what to check.
  if command -v ufw >/dev/null 2>&1 && systemctl is-active --quiet ufw 2>/dev/null; then
    printf '\n\033[1;33mufw is active on this laptop. If the phone keeps loading, allow the two ports once:\033[0m\n'
    printf '  sudo ufw allow %s/tcp && sudo ufw allow 8081/tcp\n' "$PORT"
  fi
  say_hdr "Everything is running"
  echo "  The app will reach the hub at http://$host:$PORT"
  case "$target" in
    web) cat <<MSG
  1. A browser tab opens with the app (or open http://localhost:8081).
  2. Sign in with $1. The code will be printed here in green.
  3. In another terminal:  make say TEXT="hello from the laptop"
  Ctrl+C here stops everything.

MSG
      cd "$ROOT/apps/mobile"
      EXPO_PUBLIC_HUB_URL="http://$host:$PORT" npx expo start --web ;;
    emulator) cat <<MSG
  1. The Android emulator boots (a minute), then the app opens in it.
  2. Sign in with $1. The code will be printed here in green.
  3. In another terminal:  make say TEXT="hello from the laptop"
  Ctrl+C here stops everything but the emulator; make emulator-stop closes it.

MSG
      # shellcheck disable=SC1090
      eval "$("$ROOT/scripts/emulator.sh" env)"
      "$ROOT/scripts/emulator.sh" start
      cd "$ROOT/apps/mobile"
      EXPO_PUBLIC_HUB_URL="http://$host:$PORT" npx expo start --android ;;
    phone) cat <<MSG
  1. Open Expo Go on your phone (same wifi) and scan the QR code below.
  2. Sign in with $1. The code will be printed here in green.
  3. In another terminal:  make say TEXT="hello from the laptop"
     and watch the chat list on the phone update.
  Ctrl+C here stops everything.

MSG
      cd "$ROOT/apps/mobile"
      EXPO_PUBLIC_HUB_URL="http://$host:$PORT" npx expo start ;;
  esac
}

start() {
  local email="$1"
  trap cleanup EXIT INT TERM
  start_hub
  sign_in "$email"
  set_up_agent
  run_agent
  watch_codes
  if [ "${PLAY_NO_APP:-}" = 1 ]; then
    say_hdr "Servers are running (PLAY_NO_APP=1). Ctrl+C stops them."
    wait
  else
    start_app "$email"
  fi
}

case "${1:-}" in
  "" | -h | --help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  say) shift; [ $# -ge 1 ] || die 'usage: scripts/play.sh say "text"'; say "$*" ;;
  stop) stop ;;
  *) start "$1" ;;
esac
