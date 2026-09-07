# App links

These two files are what make a scanned `https://…/p/<code>` link open the app
instead of a browser tab. Neither can be written until the app has been built
for a store, because both name identifiers that do not exist yet.

Until they are real, the `.example` files sit here unused and a scanned link
opens the pair page in a browser, which still works: it offers an
`Open in Cuckoo` button on the app's own scheme.

## Android

1. Set `android.package` in `apps/mobile/app.config.ts`, e.g. `in.cuckoo.app`.
2. Build once, then read the release signing certificate's SHA-256 fingerprint.
   With EAS: `eas credentials`. With a local keystore:
   `keytool -list -v -keystore release.keystore | grep SHA256`.
3. Copy `assetlinks.json.example` to `assetlinks.json` and fill both in.
4. Add the intent filter to `app.config.ts`:

   ```ts
   android: {
     package: 'in.cuckoo.app',
     intentFilters: [
       {
         action: 'VIEW',
         autoVerify: true,
         data: [{ scheme: 'https', host: 'cuckoo.in', pathPrefix: '/p/' }],
         category: ['BROWSABLE', 'DEFAULT'],
       },
     ],
   },
   ```

## iPhone

1. Set `ios.bundleIdentifier` in `app.config.ts`, e.g. `in.cuckoo.app`.
2. Find the team id in the Apple Developer account.
3. Copy `apple-app-site-association.example` to `apple-app-site-association`
   — **no file extension** — and fill in `<TEAMID>.<bundle id>`.
4. Add `ios: { associatedDomains: ['applinks:cuckoo.in'] }` to `app.config.ts`.

The file must be served as `application/json`. The Caddyfile in `deploy/`
already does that.

## Both

App links only work in a real build. Expo Go cannot register them, so test in a
development build or a store build.
