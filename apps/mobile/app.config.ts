import type { ExpoConfig } from 'expo/config';

// The hub the app talks to. Set EXPO_PUBLIC_HUB_URL before `expo start`:
// the laptop's wifi address for a phone, localhost for the browser,
// 10.0.2.2 for the Android emulator. make play does this.
const hubUrl = process.env.EXPO_PUBLIC_HUB_URL ?? 'http://localhost:8080';

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
  web: { bundler: 'metro', output: 'single', favicon: './assets/favicon.png' },
  plugins: ['expo-router', 'expo-secure-store'],
  extra: { hubUrl },
};

export default config;
