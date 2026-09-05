"""Drive the app in headless Chromium and take screenshots: the build loop
for anyone, person or agent, working on the app without a phone in hand.

    python3 scripts/webshot.py '<steps as a JSON list>'

It needs Chromium on the PATH and the `websockets` package; the Python
environment of any example agent has it:

    examples/echo/.venv/bin/python scripts/webshot.py "$(cat steps.json)"

Steps run in order. Each is a list whose first item names it:

  ["open", url, secs]         navigate and wait
  ["media", "dark"|"light"]   emulate the phone's colour scheme
  ["type", label, text]       focus the input with this aria-label or testID, insert text
  ["typecmd", label, cmd]     the same, typing a shell command's output (e.g. a sign-in code)
  ["click", text, secs]       click the last visible button with this text
  ["clickid", testID, secs]   click the last visible element with this testID
  ["tab", name, secs]         open a bottom tab by route name ("chats", "agents")
  ["wait", text, secs]        poll until the page's text contains text
  ["sleep", secs]
  ["shot", path]              save a PNG
  ["text"]                    print the page's text
  ["eval", js]                print what an expression evaluates to
  ["shell", cmd]              run a shell command

The window is phone-sized (390x844). On a failure a screenshot is written
next to the last one as error.png, with the page's text printed.
"""

import asyncio
import base64
import json
import os
import subprocess
import sys
import tempfile
import time
import urllib.request

import websockets

PORT = 9333
_id = 0
last_shot_dir = "."

# Expo-router on the web keeps the previous screen mounted under the current
# one, so "the last visible match" is the one on screen.
VISIBLE = "const visible = e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0; };"


async def cdp(ws, method, **params):
    global _id
    _id += 1
    await ws.send(json.dumps({"id": _id, "method": method, "params": params}))
    while True:
        msg = json.loads(await ws.recv())
        if msg.get("id") == _id:
            if "error" in msg:
                raise RuntimeError(msg["error"])
            return msg.get("result", {})


async def evaluate(ws, expr):
    r = await cdp(ws, "Runtime.evaluate", expression=expr, returnByValue=True, awaitPromise=True)
    return r.get("result", {}).get("value")


async def page_text(ws):
    return (await evaluate(ws, "document.body.innerText")) or ""


async def type_into(ws, label, text):
    ok = await evaluate(
        ws,
        f"""(() => {{ {VISIBLE}
        const els = [...document.querySelectorAll('input[aria-label={json.dumps(label)}], [data-testid={json.dumps(label)}]')].filter(visible);
        const el = els[els.length - 1];
        if (!el) return false; el.focus(); return true; }})()""",
    )
    if not ok:
        raise RuntimeError(f"no input labelled {label!r}")
    await cdp(ws, "Input.insertText", text=text)


async def click_text(ws, text):
    # Icon fonts put a private-use character before a label; ignore it.
    ok = await evaluate(
        ws,
        f"""(() => {{ {VISIBLE}
        const els = [...document.querySelectorAll('div[role="button"], button, a')].filter(visible);
        const el = els.reverse().find(e => e.textContent.replace(/[\\uE000-\\uF8FF]/g, '').trim() === {json.dumps(text)});
        if (!el) return false; el.click(); return true; }})()""",
    )
    if not ok:
        raise RuntimeError(f"no button {text!r}")


async def click_id(ws, testid):
    ok = await evaluate(
        ws,
        f"""(() => {{ {VISIBLE}
        const els = [...document.querySelectorAll('[data-testid={json.dumps(testid)}]')].filter(visible);
        const el = els[els.length - 1];
        if (!el) return false; el.click(); return true; }})()""",
    )
    if not ok:
        raise RuntimeError(f"no element with testID {testid!r}")


async def open_tab(ws, name):
    ok = await evaluate(
        ws,
        f"""(() => {{ const a = document.querySelector('a[role="tab"][href="/{name}"]');
        if (!a) return false; a.click(); return true; }})()""",
    )
    if not ok:
        raise RuntimeError(f"no tab {name!r}")


async def wait_text(ws, text, secs):
    for _ in range(int(secs * 4)):
        if text in await page_text(ws):
            return
        await asyncio.sleep(0.25)
    raise RuntimeError(f"timed out waiting for {text!r}")


async def shot(ws, path):
    global last_shot_dir
    last_shot_dir = os.path.dirname(path) or "."
    r = await cdp(ws, "Page.captureScreenshot", format="png")
    with open(path, "wb") as f:
        f.write(base64.b64decode(r["data"]))
    print("shot", path)


def wait_secs(step, i, default):
    return step[i] if len(step) > i else default


async def run_step(ws, step):
    kind = step[0]
    if kind == "open":
        await cdp(ws, "Page.navigate", url=step[1])
        await asyncio.sleep(wait_secs(step, 2, 3))
    elif kind == "media":
        await cdp(ws, "Emulation.setEmulatedMedia", features=[{"name": "prefers-color-scheme", "value": step[1]}])
    elif kind == "type":
        await type_into(ws, step[1], step[2])
    elif kind == "typecmd":
        out = subprocess.run(step[2], shell=True, capture_output=True, text=True).stdout.strip()
        print("typing", repr(out), "into", step[1])
        await type_into(ws, step[1], out)
    elif kind == "click":
        await click_text(ws, step[1])
        await asyncio.sleep(wait_secs(step, 2, 1.5))
    elif kind == "clickid":
        await click_id(ws, step[1])
        await asyncio.sleep(wait_secs(step, 2, 1.5))
    elif kind == "tab":
        await open_tab(ws, step[1])
        await asyncio.sleep(wait_secs(step, 2, 1.5))
    elif kind == "wait":
        await wait_text(ws, step[1], wait_secs(step, 2, 10))
    elif kind == "sleep":
        await asyncio.sleep(step[1])
    elif kind == "shot":
        await shot(ws, step[1])
    elif kind == "text":
        print("page text:", (await page_text(ws))[:600].replace("\n", " | "))
    elif kind == "eval":
        print("eval:", await evaluate(ws, step[1]))
    elif kind == "shell":
        out = subprocess.run(step[1], shell=True, capture_output=True, text=True)
        print("shell:", (out.stdout + out.stderr).strip()[:300])
    else:
        raise RuntimeError(f"unknown step {kind!r}")


async def main(steps):
    profile = tempfile.mkdtemp(prefix="cuckoo-webshot-")
    proc = subprocess.Popen(
        [
            "chromium",
            "--headless=new",
            "--no-sandbox",
            "--disable-gpu",
            "--hide-scrollbars",
            "--window-size=390,844",
            f"--remote-debugging-port={PORT}",
            f"--user-data-dir={profile}",
            "about:blank",
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        page = None
        for _ in range(50):
            try:
                targets = json.load(urllib.request.urlopen(f"http://127.0.0.1:{PORT}/json"))
                page = next(t for t in targets if t["type"] == "page")
                break
            except Exception:
                time.sleep(0.2)
        if page is None:
            raise RuntimeError("Chromium did not start")
        async with websockets.connect(page["webSocketDebuggerUrl"], max_size=None) as ws:
            await cdp(ws, "Page.enable")
            await cdp(ws, "Runtime.enable")
            try:
                for step in steps:
                    await run_step(ws, step)
            except Exception as e:
                await shot(ws, os.path.join(last_shot_dir, "error.png"))
                print("FAILED:", e)
                print("page text:", (await page_text(ws))[:600].replace("\n", " | "))
                return 1
    finally:
        proc.terminate()
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(2)
    sys.exit(asyncio.run(main(json.loads(sys.argv[1]))))
