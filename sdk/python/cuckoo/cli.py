"""The command line: create agents and hand them out, from a terminal.

    export CUCKOO_KEY=mgt_tok_...            # created in the app
    export CUCKOO_HUB=https://hub.example.com

    cuckoo agents list
    cuckoo agents create sbi-cards "SBI Cards"
    cuckoo agents connect agt_...            # prints the backend's secret
    cuckoo codes create agt_... --qr card.png
    cuckoo codes create agt_... --once --payload '{"customer_ref": "SBI-8812"}'
    cuckoo codes list agt_...
    cuckoo codes revoke agt_... tok_...

Every command works from a deploy script with no phone involved, which is
the point of an API key.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Any

from .agent import ProtocolError
from .management import DEFAULT_HUB, Management


def _client(args: argparse.Namespace) -> Management:
    key = args.key or os.environ.get("CUCKOO_KEY", "")
    if not key:
        raise SystemExit("no API key: set CUCKOO_KEY, or pass --key. Create one in the Cuckoo app.")
    return Management(key=key, hub=args.hub or os.environ.get("CUCKOO_HUB", DEFAULT_HUB))


def _print(value: Any) -> None:
    print(json.dumps(value, indent=2))


def _agents_list(args: argparse.Namespace) -> None:
    with _client(args) as cuckoo:
        for a in cuckoo.agents():
            print(f"{a.id}  @{a.handle:<24} {a.display_name}  [{a.status or 'no backend'}]")


def _agents_create(args: argparse.Namespace) -> None:
    with _client(args) as cuckoo:
        agent = cuckoo.create_agent(args.handle, args.display_name, args.description or "")
        print(agent.id)


def _agents_connect(args: argparse.Namespace) -> None:
    with _client(args) as cuckoo:
        secret = cuckoo.connect(args.agent, webhook_url=args.webhook)
        print(secret)


def _agents_delete(args: argparse.Namespace) -> None:
    with _client(args) as cuckoo:
        cuckoo.delete_agent(args.agent)


def _codes_create(args: argparse.Namespace) -> None:
    payload = json.loads(args.payload) if args.payload else None
    with _client(args) as cuckoo:
        code = cuckoo.create_code(
            args.agent,
            payload=payload,
            max_uses=1 if args.once else args.max_uses,
            expires_in=args.expires_in,
        )
        if args.qr:
            with open(args.qr, "wb") as f:
                f.write(code.png_bytes())
            print(f"{code.url}\nQR written to {args.qr}")
        else:
            print(code.url)


def _codes_list(args: argparse.Namespace) -> None:
    with _client(args) as cuckoo:
        for token in cuckoo.codes(args.agent):
            used = token["use_count"]
            of = token["max_uses"] or "unlimited"
            state = "revoked" if token["revoked_at"] else "live"
            print(f"{token['id']}  {used}/{of} used  {state}")


def _codes_revoke(args: argparse.Namespace) -> None:
    with _client(args) as cuckoo:
        cuckoo.revoke_code(args.agent, args.token)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="cuckoo", description=__doc__.split("\n", 1)[0])
    parser.add_argument("--key", help="API key; defaults to CUCKOO_KEY")
    parser.add_argument("--hub", help="hub base URL; defaults to CUCKOO_HUB")
    groups = parser.add_subparsers(dest="group", required=True)

    agents = groups.add_parser("agents", help="create and connect agents").add_subparsers(
        dest="command", required=True
    )
    agents.add_parser("list", help="list the account's agents").set_defaults(run=_agents_list)

    create = agents.add_parser("create", help="create an agent")
    create.add_argument("handle", help="its permanent address on the hub")
    create.add_argument("display_name", help="the name people see")
    create.add_argument("--description", help="one line about what it does")
    create.set_defaults(run=_agents_create)

    connect = agents.add_parser("connect", help="attach a backend and print its secret")
    connect.add_argument("agent")
    connect.add_argument("--webhook", help="a URL the hub posts to; omit for a socket backend")
    connect.set_defaults(run=_agents_connect)

    delete = agents.add_parser("delete", help="retire an agent")
    delete.add_argument("agent")
    delete.set_defaults(run=_agents_delete)

    codes = groups.add_parser("codes", help="hand an agent out").add_subparsers(
        dest="command", required=True
    )
    code_create = codes.add_parser("create", help="mint a code")
    code_create.add_argument("agent")
    code_create.add_argument("--once", action="store_true", help="a single-use code, per customer")
    code_create.add_argument("--max-uses", type=int, dest="max_uses")
    code_create.add_argument("--expires-in", type=int, dest="expires_in", help="seconds")
    code_create.add_argument("--payload", help="JSON handed to your backend when someone joins")
    code_create.add_argument("--qr", help="write the QR picture to this PNG file")
    code_create.set_defaults(run=_codes_create)

    code_list = codes.add_parser("list", help="list an agent's codes")
    code_list.add_argument("agent")
    code_list.set_defaults(run=_codes_list)

    code_revoke = codes.add_parser("revoke", help="stop a code working")
    code_revoke.add_argument("agent")
    code_revoke.add_argument("token")
    code_revoke.set_defaults(run=_codes_revoke)

    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        args.run(args)
    except ProtocolError as e:
        print(f"error: {e.code}: {e.message}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
