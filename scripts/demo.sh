#!/usr/bin/env bash
# Sets you up on a running hub and runs the echo agent, asking only for the
# sign-in code. Then lets you send messages as yourself from a terminal.
#
#   scripts/demo.sh you@example.com        # sign in, create the agent, run it
#   scripts/demo.sh say "hello there"      # send a message as you, from another terminal
#
# The hub prints sign-in codes to its own terminal (CUCKOO_MAIL=console);
# this script asks you to type the one you see there.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HUB="${CUCKOO_HUB:-http://localhost:8080}"
STATE="$ROOT/.demo.env"
J='Content-Type: application/json'

json() { python3 -c "import sys, json; d = json.load(sys.stdin); $1"; }

die() { echo "error: $*" >&2; exit 1; }

# fail_on_error prints the hub's error and exits if a response is an error envelope.
fail_on_error() {
  local body="$1" what="$2"
  if printf '%s' "$body" | json 'sys.exit(0 if "error" in d else 1)' 2>/dev/null; then
    printf '%s' "$body" | json 'print("%s: %s" % (d["error"]["code"], d["error"]["message"]))' >&2
    die "$what failed"
  fi
}

say() {
  [ -f "$STATE" ] || die "run 'scripts/demo.sh you@example.com' first"
  # shellcheck disable=SC1090
  . "$STATE"
  local text="$1"
  local body
  body=$(curl -s -H "X-Dev-User: $DEMO_USER_ID" -X POST "$HUB/v1/client/conversations/$DEMO_CONV_ID/messages" \
    -H "$J" -d "$(printf '%s' "$text" | python3 -c 'import sys, json; print(json.dumps({"text": sys.stdin.read()}))')")
  fail_on_error "$body" "send"
  printf '%s' "$body" | json 'm = d["message"]; print("sent %s (%s): %r" % (m["id"], m["delivery_status"], m["body"]["text"]))'
}

setup() {
  local email="$1"
  curl -sf "$HUB/healthz" >/dev/null || die "the hub is not running at $HUB (start it with: make run)"

  echo "Requesting a sign-in code for $email ..."
  local body
  body=$(curl -s -X POST "$HUB/v1/auth/email/start" -H "$J" -d "{\"email\":\"$email\"}")
  fail_on_error "$body" "requesting a code"
  echo
  echo "Look at the hub's terminal for a line containing:  Your sign-in code is ......"
  read -r -p "Type that six-digit code here: " code
  echo

  body=$(curl -s -X POST "$HUB/v1/auth/email/verify" -H "$J" \
    -d "{\"email\":\"$email\",\"code\":\"$code\",\"device_name\":\"demo script\"}")
  fail_on_error "$body" "sign-in"
  local user_id
  user_id=$(printf '%s' "$body" | json 'print(d["user"]["id"])')
  echo "Signed in as $user_id"
  local h="X-Dev-User: $user_id"

  # One echo agent per person. Handles are unique across the hub, so a new
  # one gets a random suffix.
  local agent_id
  agent_id=$(curl -s -H "$h" "$HUB/v1/mgmt/agents" | json 'a = [x for x in d["agents"] if x["display_name"] == "Echo"]; print(a[0]["id"] if a else "")')
  if [ -z "$agent_id" ]; then
    local handle="echo-$(head -c 4 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    body=$(curl -s -H "$h" -X POST "$HUB/v1/mgmt/agents" -H "$J" \
      -d "{\"handle\":\"$handle\",\"display_name\":\"Echo\",\"description\":\"Says what you said.\"}")
    fail_on_error "$body" "creating the agent"
    agent_id=$(printf '%s' "$body" | json 'print(d["agent"]["id"])')
    echo "Created agent Echo ($agent_id)"
  else
    echo "Using your existing agent Echo ($agent_id)"
  fi

  body=$(curl -s -H "$h" -X POST "$HUB/v1/mgmt/agents/$agent_id/binding" -H "$J" -d '{"mode":"socket"}')
  fail_on_error "$body" "creating the binding"
  local secret
  secret=$(printf '%s' "$body" | json 'print(d["secret"])')

  local conv_id
  conv_id=$(curl -s -H "$h" "$HUB/v1/client/conversations" | json "
c = [x for x in d['conversations'] if any(p['kind'] == 'agent' and p['id'] == '$agent_id' for p in x['participants'])]
print(c[0]['id'] if c else '')")
  [ -n "$conv_id" ] || die "could not find the chat with the agent"

  printf 'DEMO_USER_ID=%s\nDEMO_AGENT_ID=%s\nDEMO_CONV_ID=%s\n' "$user_id" "$agent_id" "$conv_id" > "$STATE"

  local echo_dir="$ROOT/examples/echo"
  if [ ! -x "$echo_dir/.venv/bin/python" ]; then
    echo "Setting up a Python environment for the agent (once) ..."
    python3 -m venv "$echo_dir/.venv"
    "$echo_dir/.venv/bin/pip" install -q -e "$ROOT/sdk/python"
  fi

  echo
  echo "Ready. Next:"
  echo "  1. Sign in on the phone with $email (a new code appears in the hub's terminal)."
  echo "  2. From another terminal, send a message as yourself:"
  echo "       scripts/demo.sh say \"hello from the laptop\""
  echo
  echo "Running the echo agent now. Press Ctrl+C to stop it."
  cd "$echo_dir"
  CUCKOO_SECRET="$secret" CUCKOO_HUB="$HUB" exec .venv/bin/python echo.py
}

case "${1:-}" in
  "" | -h | --help) sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  say) shift; [ $# -ge 1 ] || die 'usage: scripts/demo.sh say "text"'; say "$*" ;;
  *) setup "$1" ;;
esac
