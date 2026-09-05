# buttons

An agent that asks before it acts. Say anything and it answers with two
buttons; tap one and it replies to your tap, quoting it.

Set up an agent with a socket binding exactly as in [`../echo`](../echo),
then:

```bash
CUCKOO_SECRET=bnd_sec_... python buttons.py
```

Tap a button from the command line the way the app will:

```bash
curl -s -H "$H" -X POST localhost:8080/v1/client/conversations/cnv_.../messages \
  -H 'Content-Type: application/json' \
  -d '{"action":{"button_id":"acc-salary","source_message_id":"msg_..."}}'
```
