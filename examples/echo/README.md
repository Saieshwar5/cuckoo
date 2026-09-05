# echo

The smallest useful agent: it replies with what it was told. It is the
reference every other backend copies, and the smoke test for the protocol.

```bash
# 1. Run a hub (see the repository README) and create an agent.
export H="X-Dev-User: usr_..."
curl -s -H "$H" -X POST localhost:8080/v1/mgmt/agents \
  -H 'Content-Type: application/json' -d '{"handle":"echo","display_name":"Echo"}'

# 2. Give it a socket binding. The secret is shown once.
curl -s -H "$H" -X POST localhost:8080/v1/mgmt/agents/agt_.../binding \
  -H 'Content-Type: application/json' -d '{"mode":"socket"}'

# 3. Run the agent.
python -m venv .venv && . .venv/bin/activate
pip install -e ../../sdk/python
CUCKOO_SECRET=bnd_sec_... python echo.py

# 4. Talk to it.
curl -s -H "$H" -X POST localhost:8080/v1/client/conversations/cnv_.../messages \
  -H 'Content-Type: application/json' -d '{"text":"hello"}'
```

Stop the script, send three more messages, start it again: all three arrive,
in order, and are answered.
