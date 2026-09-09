import * as Notifications from 'expo-notifications';
import { useRouter } from 'expo-router';
import { useEffect } from 'react';

import { useSession } from '@/session/SessionProvider';

import { conversationOf, registerForPush } from './push';

/**
 * Registers this device once somebody is signed in, and opens the right chat
 * when a notification is tapped.
 *
 * Registration is tied to being signed in because the address belongs to a
 * session: it is how the hub knows which device to wake, and signing out ends
 * both at once.
 */
export function usePush() {
  const { status, api } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (status !== 'signedIn') return;
    registerForPush(api).catch(() => {
      // Being refused, or having no way to ask, is an ordinary answer. The
      // app works without notifications; it simply cannot reach anybody first.
    });
  }, [status, api]);

  useEffect(() => {
    // A tap while the app is running.
    const tapped = Notifications.addNotificationResponseReceivedListener((response) => {
      const id = conversationOf(response);
      if (id) router.push({ pathname: '/chat/[id]', params: { id } });
    });

    // And a tap that started it: the notification that was open before any of
    // this existed. Without it, opening from a locked phone lands on the chat
    // list and the person has to find the message themselves.
    void Notifications.getLastNotificationResponseAsync().then((response) => {
      if (!response) return;
      const id = conversationOf(response);
      if (id) router.push({ pathname: '/chat/[id]', params: { id } });
    });

    return () => tapped.remove();
  }, [router]);
}
