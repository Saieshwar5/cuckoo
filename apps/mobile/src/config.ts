import Constants from 'expo-constants';

// Where the hub is. Comes from app.config.ts, which reads CUCKOO_HUB_URL.
export const hubUrl: string =
  (Constants.expoConfig?.extra?.hubUrl as string | undefined) ?? 'http://localhost:8080';

export const socketUrl = `${hubUrl.replace(/^http/, 'ws')}/v1/client/socket`;
