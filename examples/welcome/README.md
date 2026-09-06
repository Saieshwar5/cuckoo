# welcome

The greeter every new account meets first. Run it against an agent whose
handle the hub was started with (`CUCKOO_WELCOME_HANDLE=welcome`), and the
hub adds that agent to each new account and tells it someone joined.

```bash
cuckoo agents create welcome "Cuckoo" --description "Says hello, and how this works." \
  --starter "How do I add an agent?" --starter "How do I make my own?"
cuckoo agents connect <agent id>          # prints the secret
CUCKOO_SECRET=bnd_sec_... python welcome.py
```

`make play` does all of this on a development hub.
