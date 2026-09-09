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
  userInterfaceStyle: 'automatic',
  ios: { supportsTablet: false },
  android: {
    // Permanent: an app's package name cannot change once it is published,
    // and everything Google knows about it hangs off this string.
    package: 'onl.cuckoo.app',
    // What tells the app which Firebase project delivers its notifications.
    // Not in the repository: it identifies this app to Google.
    googleServicesFile: './google-services.json',
    adaptiveIcon: {
      backgroundColor: '#000000',
      foregroundImage: './assets/android-icon-foreground.png',
      backgroundImage: './assets/android-icon-background.png',
      monochromeImage: './assets/android-icon-monochrome.png',
    },
    predictiveBackGestureEnabled: false,
  },
  web: { bundler: 'metro', output: 'single', favicon: './assets/favicon.png' },
  plugins: [
    'expo-router',
    'expo-secure-store',
    ['expo-camera', { cameraPermission: 'Cuckoo uses the camera to scan agent codes.' }],
    [
      'expo-notifications',
      {
        // Android tints the small icon itself, so it must be a silhouette on
        // transparency — which is exactly what the monochrome adaptive icon
        // already is. One drawing, two places it is right.
        icon: './assets/android-icon-monochrome.png',
        color: '#000000',
      },
    ],
    [
      'expo-splash-screen',
      {
        image: './assets/splash-icon.png',
        imageWidth: 160,
        resizeMode: 'contain',
        backgroundColor: '#000000',
      },
    ],
  ],
  extra: { hubUrl },
};

export default config;
