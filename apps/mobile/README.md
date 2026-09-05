# Cuckoo mobile

The app. React Native with Expo, TypeScript, file-based routing.

```bash
npm install
CUCKOO_HUB_URL=http://192.168.1.10:8080 npx expo start   # the laptop running the hub
```

Open it in Expo Go on a phone on the same wifi. Sign in with any email;
the code is printed in the hub's log (`CUCKOO_MAIL=console`).

```bash
npm run check      # typecheck, lint, format, tests
```

Layout: `app/` is the route tree; `src/` is everything a route uses —
the API client, the session, the live socket, the chat list reducer, the
theme tokens and the strings. Screens hold no rules; the modules under
`src/` do, and those are what the tests cover.
