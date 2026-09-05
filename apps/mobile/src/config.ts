import Constants from 'expo-constants';

// Where the hub is. EXPO_PUBLIC_HUB_URL is inlined at build time on every
// platform, which is the one mechanism that works for a phone, the browser
// and the emulator alike; the config's extra is the fallback.
export const hubUrl: string =
  process.env.EXPO_PUBLIC_HUB_URL ??
  (Constants.expoConfig?.extra?.hubUrl as string | undefined) ??
  'http://localhost:8080';

export const socketUrl = `${hubUrl.replace(/^http/, 'ws')}/v1/client/socket`;
