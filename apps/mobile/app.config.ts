import type { ExpoConfig } from 'expo/config';

// The hub the app talks to. On a phone on the same wifi as a laptop running
// the hub, set CUCKOO_HUB_URL to the laptop's address before `expo start`.
const hubUrl = process.env.CUCKOO_HUB_URL ?? 'http://localhost:8080';

const config: ExpoConfig = {
  name: 'Cuckoo',
  slug: 'cuckoo',
  version: '0.1.0',
  scheme: 'cuckoo',
  orientation: 'portrait',
  icon: './assets/icon.png',
  userInterfaceStyle: 'light',
  ios: { supportsTablet: false },
  android: {
    adaptiveIcon: {
      backgroundColor: '#FFFFFF',
      foregroundImage: './assets/android-icon-foreground.png',
      backgroundImage: './assets/android-icon-background.png',
      monochromeImage: './assets/android-icon-monochrome.png',
    },
    predictiveBackGestureEnabled: false,
  },
  plugins: ['expo-router', 'expo-secure-store'],
  extra: { hubUrl },
};

export default config;
