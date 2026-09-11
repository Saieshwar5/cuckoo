import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { deviceZone } from '@/schedules/format';

import { useSession } from './SessionProvider';

/**
 * Tells the hub where this phone's clock is, when somebody is signed in: at
 * start, and again whenever the app comes back to the front in another zone.
 *
 * Agents read "tomorrow at 7" on it, and the person's schedules move with it
 * — land in London and "every morning at 7" is seven in London. Once per
 * zone per run of the app: the hub does nothing with a zone it already has.
 */
export function useTimezone() {
  const { status, api } = useSession();
  const reported = useRef<string | null>(null);

  useEffect(() => {
    if (status !== 'signedIn') {
      reported.current = null;
      return;
    }
    const report = () => {
      const zone = deviceZone();
      if (zone === reported.current) return;
      reported.current = zone;
      api.updateMe({ timezone: zone }).catch(() => {
        // Tried again next time the app comes to the front.
        reported.current = null;
      });
    };
    report();
    const sub = AppState.addEventListener('change', (next) => {
      if (next === 'active') report();
    });
    return () => sub.remove();
  }, [status, api]);
}
