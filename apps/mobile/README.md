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

Layout: `app/` is the route tree; `src/` is everything a route uses —
the API client, the session, the live socket, the chat list reducer, the
theme tokens and the strings. Screens hold no rules; the modules under
`src/` do, and those are what the tests cover.
