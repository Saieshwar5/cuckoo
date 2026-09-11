import { useCallback, useEffect, useState } from 'react';

import type { Cadence, Schedule } from '../api/types';
import { useRealtime } from '../realtime/RealtimeProvider';
import { useSession } from '../session/SessionProvider';
import { applyScheduleFrame, removeSchedule, upsertSchedule } from './store';

// useSchedules is one conversation's schedules, loaded from the hub and kept
// live, with what the person can do to them. Every change is the hub's
// answer, not a guess: a schedule shows "waiting" until the agent confirms.
export function useSchedules(conversationId: string) {
  const { api } = useSession();
  const realtime = useRealtime();
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  // Asking again after the connection comes back: whatever changed while
  // it was down is in the hub and not in any frame.
  const reload = useCallback(async () => {
    try {
      setSchedules(await api.listSchedules(conversationId));
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [api, conversationId]);

  useEffect(() => {
    let cancelled = false;
    api.listSchedules(conversationId).then(
      (list) => {
        if (cancelled) return;
        setSchedules(list);
        setLoading(false);
      },
      (e: unknown) => {
        if (cancelled) return;
        setError(e);
        setLoading(false);
      },
    );
    const unsubscribe = realtime?.subscribe({
      onFrame: (frame) => setSchedules((list) => applyScheduleFrame(list, frame, conversationId)),
      onOpen: (reconnect) => {
        if (reconnect) void reload();
      },
    });
    return () => {
      cancelled = true;
      unsubscribe?.();
    };
  }, [api, realtime, conversationId, reload]);

  const request = async (input: { instruction: string; cadence: Cadence }) => {
    const made = await api.requestSchedule(conversationId, input);
    setSchedules((list) => upsertSchedule(list, made));
    return made;
  };
  const update = async (
    id: string,
    change: { paused?: boolean; instruction?: string; cadence?: Cadence },
  ) => {
    const changed = await api.updateSchedule(conversationId, id, change);
    setSchedules((list) => upsertSchedule(list, changed));
    return changed;
  };
  const remove = async (id: string) => {
    await api.deleteSchedule(conversationId, id);
    setSchedules((list) => removeSchedule(list, id));
  };

  return { schedules, loading, error, reload, request, update, remove };
}
