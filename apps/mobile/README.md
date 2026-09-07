# Cuckoo mobile

The app. React Native with Expo, TypeScript, file-based routing.

```bash
npm install
npx expo start --web                                    # in the laptop's browser, against localhost:8080
EXPO_PUBLIC_HUB_URL=http://192.168.1.10:8080 npx expo start   # on a phone: the laptop's wifi address
```

The browser is for building: every change shows up in a tab on the laptop.
Use the browser's device toolbar for a phone-sized view. The phone, or the
Android emulator (`make emulator`), is for checking how it actually feels.
Sign in with any email; the code is printed in the hub's log
(`CUCKOO_MAIL=console`).

```bash
npm run check      # typecheck, lint, format, tests
```

## Seeing it while building

The browser is the build loop. Start everything with `make web EMAIL=...`
from the repository root, or by hand:

```bash
EXPO_PUBLIC_HUB_URL=http://localhost:8080 npx expo start --web
```

Every saved change reloads the tab. For a screenshot without a window, for
instance from an agent doing the building:

```bash
chromium --headless=new --window-size=390,844 --screenshot=shot.png http://localhost:8081/
```

Two things differ in a browser and are handled: the session lives in
localStorage instead of the keychain (`src/session/store.web.ts`), and the
live socket carries the token as the `cuckoo` subprotocol because browsers
cannot set headers on a WebSocket. The hub allows any origin in
development (`CUCKOO_CORS_ORIGINS`).

For how it feels on Android, `make emulator` boots the Android emulator
and `make play EMAIL=... TARGET=emulator` opens the app in it.

## The look

Dark first, following the phone's setting, and only white, grey and black:
our messages are ink on paper, the agent's are paper on ink, avatars are
shades of grey, and the one thing that gets a colour is an error. The
layout is the one people already know from their chat apps: a floating
action button, tabs with a tinted pill behind the active one, bubbles with
a tail, a pill between days. The mark is a bird whose body is a speech
bubble (`assets/logo-*.png`, `src/components/Logo.tsx`); it is the app
icon, the splash, the favicon, and sits beside the name on the chat list.
`src/theme/tokens.ts` holds both palettes and the type scale. `ThemeProvider` at the root reads the phone's setting once;
screens take colours from `useTheme()` and build their styles with
`useStyles(make)`, so no screen knows which theme is on. The platform's own
typeface is used on purpose: Roboto on Android, San Francisco on iOS.

The components under `src/components` are the vocabulary: `Screen`,
`Header`, `SearchBar`, `Fab`, `EmptyState`, `Button` (primary, outline,
plain), `TextField`, `CodeInput`, `Avatar`, `IconButton`, `ChatRow`, and
under `chat/` the conversation's parts: `ChatHeader`, `Bubble` (text, quote,
buttons, time and ticks, the streaming caret), `TypingBubble`, `DayDivider`,
`QuickReplies`, `Composer`. A screen composes those and holds no rules.

## The conversation

`app/chat/[id].tsx` is an inverted list over `useChat(id)`: newest at the
bottom where the eye is, older pages loading above without moving what is
on screen. `src/chat/store.ts` is the pure reducer (paging, live frames,
streaming deltas, typing with expiry, our own sends before the hub confirms
them) and `src/chat/controller.ts` drives it: load, catch up after a
reconnect from the newest message it has, send with an idempotency key and
retry a failure with the same key. One live connection per session
(`src/realtime/realtime.ts`, opened by `RealtimeProvider`) feeds both the
chat list and the open chat.

## Faces

`src/components/Avatar.tsx` draws a photo when there is one and the name's
initials when there is not, everywhere: the chat list, the chat header, the
Agents tab, an agent's profile, and the card a scanned code resolves to.

An agent's picture is public, so it is a plain image source built from its
id (`src/media/avatar.ts`) — no token, no fetching dance. A person's photo
goes through `useMediaSource` behind their session, like anything else they
own.

`AvatarPicker` is the disc with a camera on it. What it hands back is the
picked file, not an uploaded one: uploading happens when the screen is
saved, so someone who chooses a photo and then leaves has not left a stray
file on the hub. `app/me.tsx` is the same control for your own photo and
name.

## Remembering, and sending with no signal

The hub is the record; the app keeps a copy of the parts it has seen. The
chat list, every chat that was opened, and the agents are written to a
store on the device as they arrive (`src/cache`), and read back before the
hub is asked the next time. A screen shows what it remembered on its first
frame and corrects it the moment the hub answers; with no signal it shows
what it remembered for as long as that lasts. Pictures already looked at
are kept too (`src/media/source.ts`). Signing out wipes all of it.

The store is a handful of JSON documents by key, one per thing a screen
loads, capped at five hundred messages a chat. SQLite on the phone,
IndexedDB in the browser, behind one four-method interface, guarded so a
device that cannot remember runs the app as it was rather than a broken
one.

A send is written to a queue (`src/outbox`) before anything else happens
to it, and shows in the chat with the pending tick. The queue drains in
order whenever the hub can be reached: at once, when the socket comes
back, when the platform says the network is back, and on a short backoff
besides. Because every send carries an idempotency key, one that was
halfway through when the app was killed goes again on the next open and
lands exactly once. A send the hub refuses is marked failed and waits for
a tap; one the hub could not be reached for stays pending, which is the
truth of it.

## Photos and files

The clip on the composer opens photos, the camera, or a document. What is
picked appears as a chip above the input and the message goes with it; a
picture needs no caption.

A photo is shrunk to 2048 pixels on its long side before it leaves the phone
(`src/media/pick.ts`). That is more than any screen shows, saves the wait on a
mobile connection, and drops the location and camera details the original
carried, because the copy is re-encoded.

Bubbles draw the small copy the hub made, never the original
(`src/components/chat/Attachments.tsx`); the full picture is fetched only when
one is opened. The bytes are behind the session token, so they cannot be a
plain `<img src>`: `src/media/source.ts` passes the header on a phone, and
`source.web.ts` fetches and hands the tag a blob URL, released when it leaves
the screen. Saving a file works the same way, split by platform in
`src/media/save.ts`.

Our own send appears before it has left: the bubble draws the file from this
device while it uploads. A failed send keeps what it already uploaded, so a
retry finishes the message rather than sending the photo twice.

## Voice notes

The send button is a microphone when nothing is typed. Tap it and the
composer becomes a recording bar — a red dot, a timer, live bars — with a
send and a bin. Ten minutes is the limit, and a note that reaches it ends
itself rather than being refused when the words are already spoken.

The waveform is measured while recording (`src/media/record.ts`), ten times
a second, and thinned to 56 bars that travel with the file. Nothing decodes
audio to draw a bubble: a chat full of voice notes scrolls like a chat full
of words. How long a note is comes from the number of samples taken, not
from the recorder's clock, because not every platform reports one.

Playback is `src/components/chat/VoiceNote.tsx`: play, the bars filling as
it goes, a tap anywhere on them to seek, and 1x / 1.5x / 2x — the only
speeds anyone wants on speech.

Holding the microphone to talk and letting go to send is deliberately not
here. Telling a hold from a tap means timing the release against the
recorder's clock, which only moves when the microphone is next polled, so
the same gesture read as a hold one time and a tap the next.

## Agents

The Agents tab lists the agents you own, live: `AgentsProvider` keeps
`AgentsController` alive for the session and the hub's `agent.status`
frames move the dot beside each one the moment a backend attaches, drops,
or stops answering. The same frame updates the chat list's avatars.

`app/agent/new` creates one (name, a handle suggested from it, a line
about it), `app/agent/[id]` is its profile, `app/agent/[id]/edit` changes
the name and description (never the handle, which is its address), and
`app/agent/[id]/connect` is the whole developer experience: pick socket or
webhook, generate the secret (shown once), copy the three-line SDK snippet
with the secret and this hub's address filled in, and watch the status
line flip to Connected when the code speaks.

## API keys

The key icon on the Agents tab opens `app/api-keys.tsx`: create a key
with a name, see it once with a copy button, watch when each was last
used, and revoke. These routes live on the client API, behind the
person's own sign-in, so a key can never make another key.

## Devices

Signing in somewhere new no longer signs the old device out. Settings →
Devices (`app/devices.tsx`) lists where the account is signed in: this
device first and marked, each with when it was last heard from, and a
way to sign out one of the others or all of them at once.
`src/devices/format.ts` writes those lines over `timeSince` in
`src/util/time.ts`, which is the app's one measure of how long ago
something was.

## Handing an agent out, and taking one in

An owner's profile has **Share**: it mints one static code for the agent
(the hub shows a code once, so the phone keeps it in the keychain, or
localStorage on the web, and asks the hub to draw the picture again on a
later visit), shows the QR and the link, counts the people who used it,
and **Stop sharing** withdraws it. The Chats button offers **Scan a code**
(the camera on a phone, and a paste-a-link field that is also how the
browser gets by) and **Create an agent**. A scanned or opened link lands
on `app/p/[code]`: the agent's card with its owner's name and the
Unverified label, and one button. Adding opens the chat, and the chat
list reloads because the hub's frames name a conversation it has not
seen. Added agents sit under **Added** on the Agents tab; their profile
has **Block**, which closes the chat both ways and puts a banner where the
composer was, with Unblock on it. `AgentsController` holds the contacts
beside the owned agents and applies the same status frames to both.

Long-press a bubble to reply to it. A tap on an agent's button sends the
choice as an action; quick replies are chips above the composer, shown while
the agent's question is the last word. In a browser, Enter sends and
Shift+Enter makes a new line.

To see the other theme in the browser: DevTools → Rendering → "Emulate CSS
media feature prefers-color-scheme". Without a window, `scripts/webshot.py`
drives headless Chromium through a list of steps (open, type, click, wait,
media, shot) and is how the screenshots for a change get made:

```bash
examples/echo/.venv/bin/python scripts/webshot.py '[
  ["open", "http://localhost:8081/", 8], ["wait", "Sign in", 60],
  ["shot", "signin-dark.png"], ["media", "light"], ["sleep", 1], ["shot", "signin-light.png"]
]'
```

Layout: `app/` is the route tree; `src/` is everything a route uses —
the API client, the session, the live socket, the chat list reducer, the
theme tokens and the strings. Screens hold no rules; the modules under
`src/` do, and those are what the tests cover.
