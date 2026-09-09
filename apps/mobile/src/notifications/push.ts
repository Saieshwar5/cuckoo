import Constants from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { Platform } from 'react-native';

import type { Api } from '@/api/client';

// Where a phone can be reached when nobody is looking at it, and what happens
// when somebody taps what arrives.
//
// Nothing here works on a simulator or in Expo Go: only a real device gets an
// address from Google, and only a build of this app can ask for one. That is
// the reason a development build exists at all.

// The channel Android draws these with. It must match what the hub sends, or
// the notification arrives with default behaviour and no sound.
const CHANNEL_ID = 'messages';

// A notification arriving while the app is open is not drawn over the top: the
// chat is already on screen, and the hub does not send one to somebody it
// knows is watching. This covers the moment in between.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: false,
    shouldShowList: true,
    shouldPlaySound: false,
    shouldSetBadge: false,
  }),
});

/**
 * Ask for permission, get this device's address, and give it to the hub.
 *
 * Returns quietly when there is nothing to do — a simulator, a browser, or a
 * person who said no. Being refused is an ordinary answer, not an error: the
 * app works without notifications, it just cannot reach anybody first.
 */
export async function registerForPush(api: Api): Promise<string | null> {
  if (!Device.isDevice) return null;

  if (Platform.OS === 'android') {
    // Declared before asking: Android wants the channel to exist so the
    // permission prompt can describe what it is for.
    await Notifications.setNotificationChannelAsync(CHANNEL_ID, {
      name: 'Messages',
      importance: Notifications.AndroidImportance.HIGH,
      lockscreenVisibility: Notifications.AndroidNotificationVisibility.PRIVATE,
      vibrationPattern: [0, 250, 250, 250],
    });
  }

  const existing = await Notifications.getPermissionsAsync();
  let status = existing.status;
  if (status !== 'granted') {
    // On Android 13 and later this is a real prompt a person can refuse, and
    // many do. Asked once, here, rather than on every launch.
    status = (await Notifications.requestPermissionsAsync()).status;
  }
  if (status !== 'granted') return null;

  const projectId = projectIdOf();
  if (!projectId) return null;

  const token = (await Notifications.getExpoPushTokenAsync({ projectId })).data;
  if (!token) return null;

  await api.registerPush(token, Platform.OS === 'ios' ? 'ios' : 'android');
  return token;
}

/** Stop this device being reachable. What signing out does. */
export async function forgetPush(): Promise<void> {
  try {
    await Notifications.unregisterForNotificationsAsync();
  } catch {
    // Best effort. Signing out revokes the session on the hub, which is what
    // actually stops the notifications; this is only tidiness.
  }
}

/**
 * What a tap opens.
 *
 * The hub puts the conversation in the notification's data, so a tap goes
 * straight to the chat rather than to whatever screen was last open.
 */
export function conversationOf(response: Notifications.NotificationResponse): string | null {
  const data = response.notification.request.content.data as { conversation_id?: unknown };
  return typeof data?.conversation_id === 'string' ? data.conversation_id : null;
}

/**
 * The EAS project this build belongs to; Expo mints tokens against it.
 *
 * Written into the build by EAS, so it is absent in Expo Go and in a browser
 * — which is one of the reasons neither can receive a notification.
 */
function projectIdOf(): string | undefined {
  const config = Constants.expoConfig as { extra?: { eas?: { projectId?: string } } } | null;
  const eas = Constants.easConfig as { projectId?: string } | null;
  return config?.extra?.eas?.projectId ?? eas?.projectId ?? undefined;
}
