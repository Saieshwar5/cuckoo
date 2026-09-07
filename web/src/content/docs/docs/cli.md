---
title: Command line
description: The cuckoo command, for creating agents and minting codes without a phone.
---

The `cuckoo` command ships with the Python SDK. It covers the management API,
which is everything except running your backend.

```bash
pip install -e sdk/python
```

## Credentials

```bash
export CUCKOO_KEY=mgt_tok_...
export CUCKOO_HUB=https://cuckoo.in
```

Both can be given as flags instead, as `--key` and `--hub`. Without a key the
command stops and says so. The hub defaults to the public one,
`https://cuckoo.in`; a development hub is `http://localhost:8080`, so set it.

An error prints as `error: <code>: <message>` and exits with status 1.

## Agents

```bash
cuckoo agents list
cuckoo agents create HANDLE "DISPLAY NAME" [--description TEXT] [--avatar PATH] [--starter TEXT ...]
cuckoo agents avatar AGENT PATH
cuckoo agents connect AGENT [--webhook URL]
cuckoo agents delete AGENT
```

`list` prints one line per agent with its id, handle, name and backend state.
`create` prints the new id and nothing else, so it pipes cleanly.

`--starter` may be repeated, up to four times.

**`connect` prints the binding secret on stdout.** It is shown once. Capture it
rather than reading it off the screen:

```bash
SECRET=$(cuckoo agents connect agt_...)
```

Running `connect` again replaces the binding and invalidates the previous
secret.

## Codes

```bash
cuckoo codes create AGENT [--once] [--max-uses N] [--expires-in SECONDS] [--payload JSON] [--qr FILE.png]
cuckoo codes list AGENT
cuckoo codes revoke AGENT TOKEN
```

A poster code, with the QR written to a file:

```bash
cuckoo codes create agt_... --qr poster.png
```

A code for one customer, good for ten minutes:

```bash
cuckoo codes create agt_... --once --expires-in 600 \
  --payload '{"customer_ref":"SBI-8812"}'
```

`--once` is the same as `--max-uses 1`, and wins if you pass both. `list` shows
each token's id, how many uses it has had, and whether it is still live. The
code itself is never listed, because the hub keeps only its hash.

## What the command does not do

- There is no way to rename an agent or change its description. Use
  `Management.update_agent` from Python.
- There is no JSON output flag. Every command prints lines for a person.
- There is no `cuckoo login`. Getting your first API key still needs the app,
  which is being fixed.
- There is no `cuckoo run`. You run your own Python file.
